// Package audit provides audit logging functionality with async buffered writes
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// Action constants
const (
	// Project actions
	ActionProjectCreate = "project.create"
	ActionProjectUpdate = "project.update"
	ActionProjectDelete = "project.delete"

	// Test run actions
	ActionRunCreate = "run.create"
	ActionRunStart  = "run.start"
	ActionRunCancel = "run.cancel"
	ActionRunDelete = "run.delete"

	// User actions
	ActionUserCreate     = "user.create"
	ActionUserUpdate     = "user.update"
	ActionUserDeactivate = "user.deactivate"
	ActionUserLogin      = "user.login"
	ActionUserLogout     = "user.logout"
	ActionUserLoginFail  = "user.login_failed"

	// Member actions
	ActionMemberInvite = "member.invite"
	ActionMemberAccept = "member.accept"
	ActionMemberRemove = "member.remove"
	ActionMemberUpdate = "member.update"

	// API key actions
	ActionAPIKeyCreate = "api_key.create"
	ActionAPIKeyRevoke = "api_key.revoke"
	ActionAPIKeyUse    = "api_key.use"

	// Tenant actions
	ActionTenantCreate = "tenant.create"
	ActionTenantUpdate = "tenant.update"
	ActionTenantDelete = "tenant.delete"

	// Billing actions
	ActionSubscriptionCreate = "subscription.create"
	ActionSubscriptionUpdate = "subscription.update"
	ActionSubscriptionCancel = "subscription.cancel"
	ActionPaymentSuccess     = "payment.success"
	ActionPaymentFailed      = "payment.failed"
)

// ResourceType constants
const (
	ResourceProject      = "project"
	ResourceTestRun      = "testrun"
	ResourceTestCase     = "testcase"
	ResourceUser         = "user"
	ResourceMembership   = "membership"
	ResourceAPIKey       = "api_key"
	ResourceTenant       = "tenant"
	ResourceSubscription = "subscription"
	ResourcePayment      = "payment"
)

// Entry represents an audit log entry
type Entry struct {
	ID           uuid.UUID         `json:"id" db:"id"`
	TenantID     uuid.UUID         `json:"tenant_id" db:"tenant_id"`
	UserID       *uuid.UUID        `json:"user_id,omitempty" db:"user_id"`
	Action       string            `json:"action" db:"action"`
	ResourceType string            `json:"resource_type" db:"resource_type"`
	ResourceID   *uuid.UUID        `json:"resource_id,omitempty" db:"resource_id"`
	Changes      json.RawMessage   `json:"changes,omitempty" db:"changes"`
	IPAddress    *string           `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent    *string           `json:"user_agent,omitempty" db:"user_agent"`
	RequestID    *string           `json:"request_id,omitempty" db:"request_id"`
	APIKeyID     *uuid.UUID        `json:"api_key_id,omitempty" db:"api_key_id"`
	Metadata     map[string]any    `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
}

// Changes represents before/after states for an audit entry
type Changes struct {
	Before map[string]interface{} `json:"before,omitempty"`
	After  map[string]interface{} `json:"after,omitempty"`
}

// LoggerConfig holds configuration for the audit logger
type LoggerConfig struct {
	// Buffer settings
	BufferSize    int           // Max entries to buffer before flush
	FlushInterval time.Duration // Time interval for flushing buffer

	// Retention
	RetentionDays int // Days to retain audit logs (0 = forever)
}

// DefaultLoggerConfig returns sensible defaults
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		BufferSize:    100,
		FlushInterval: 1 * time.Second,
		RetentionDays: 90,
	}
}

// Logger provides async buffered audit logging
type Logger struct {
	db     *sqlx.DB
	config LoggerConfig
	logger *zap.Logger

	// Buffer for async writes
	buffer chan *Entry
	wg     sync.WaitGroup
	done   chan struct{}
}

// NewLogger creates a new audit logger
func NewLogger(db *sqlx.DB, config LoggerConfig, logger *zap.Logger) *Logger {
	if config.BufferSize == 0 {
		config.BufferSize = DefaultLoggerConfig().BufferSize
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = DefaultLoggerConfig().FlushInterval
	}

	l := &Logger{
		db:     db,
		config: config,
		logger: logger,
		buffer: make(chan *Entry, config.BufferSize*2),
		done:   make(chan struct{}),
	}

	// Start background writer
	l.wg.Add(1)
	go l.backgroundWriter()

	return l
}

// Log writes an audit entry asynchronously
func (l *Logger) Log(ctx context.Context, entry *Entry) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// Non-blocking send to buffer
	select {
	case l.buffer <- entry:
		// Sent successfully
	default:
		// Buffer full, log warning and try direct write
		l.logger.Warn("audit buffer full, writing directly",
			zap.String("action", entry.Action),
			zap.String("resource_type", entry.ResourceType),
		)
		go l.writeEntry(ctx, entry)
	}
}

// LogSync writes an audit entry synchronously
func (l *Logger) LogSync(ctx context.Context, entry *Entry) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	return l.writeEntry(ctx, entry)
}

// backgroundWriter continuously flushes the buffer
func (l *Logger) backgroundWriter() {
	defer l.wg.Done()

	batch := make([]*Entry, 0, l.config.BufferSize)
	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry := <-l.buffer:
			batch = append(batch, entry)
			if len(batch) >= l.config.BufferSize {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-l.done:
			// Drain remaining entries
			close(l.buffer)
			for entry := range l.buffer {
				batch = append(batch, entry)
			}
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch writes a batch of entries to the database
func (l *Logger) flushBatch(batch []*Entry) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build bulk insert
	query := `INSERT INTO audit_logs (
		id, tenant_id, user_id, action, resource_type, resource_id,
		changes, ip_address, user_agent, request_id, api_key_id, metadata, created_at
	) VALUES `

	args := make([]interface{}, 0, len(batch)*13)
	values := make([]string, 0, len(batch))

	for i, entry := range batch {
		base := i * 13
		values = append(values, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12, base+13,
		))

		metadata, _ := json.Marshal(entry.Metadata)

		args = append(args,
			entry.ID,
			entry.TenantID,
			entry.UserID,
			entry.Action,
			entry.ResourceType,
			entry.ResourceID,
			entry.Changes,
			entry.IPAddress,
			entry.UserAgent,
			entry.RequestID,
			entry.APIKeyID,
			metadata,
			entry.CreatedAt,
		)
	}

	query += fmt.Sprintf("%s ON CONFLICT DO NOTHING", values[0])
	for i := 1; i < len(values); i++ {
		query = query[:len(query)-len(" ON CONFLICT DO NOTHING")] + ", " + values[i] + " ON CONFLICT DO NOTHING"
	}

	_, err := l.db.ExecContext(ctx, query, args...)
	if err != nil {
		l.logger.Error("failed to flush audit batch",
			zap.Error(err),
			zap.Int("batch_size", len(batch)),
		)
		return
	}

	l.logger.Debug("flushed audit batch",
		zap.Int("count", len(batch)),
	)
}

// writeEntry writes a single entry to the database
func (l *Logger) writeEntry(ctx context.Context, entry *Entry) error {
	metadata, _ := json.Marshal(entry.Metadata)

	query := `INSERT INTO audit_logs (
		id, tenant_id, user_id, action, resource_type, resource_id,
		changes, ip_address, user_agent, request_id, api_key_id, metadata, created_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := l.db.ExecContext(ctx, query,
		entry.ID,
		entry.TenantID,
		entry.UserID,
		entry.Action,
		entry.ResourceType,
		entry.ResourceID,
		entry.Changes,
		entry.IPAddress,
		entry.UserAgent,
		entry.RequestID,
		entry.APIKeyID,
		metadata,
		entry.CreatedAt,
	)

	return err
}

// Close gracefully shuts down the logger
func (l *Logger) Close() error {
	close(l.done)
	l.wg.Wait()
	return nil
}

// Query returns audit entries matching the criteria
func (l *Logger) Query(ctx context.Context, opts QueryOptions) ([]*Entry, error) {
	query := `SELECT id, tenant_id, user_id, action, resource_type, resource_id,
		changes, ip_address, user_agent, request_id, api_key_id, metadata, created_at
		FROM audit_logs WHERE tenant_id = $1`

	args := []interface{}{opts.TenantID}
	argNum := 2

	if opts.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argNum)
		args = append(args, *opts.UserID)
		argNum++
	}

	if opts.ResourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", argNum)
		args = append(args, opts.ResourceType)
		argNum++
	}

	if opts.ResourceID != nil {
		query += fmt.Sprintf(" AND resource_id = $%d", argNum)
		args = append(args, *opts.ResourceID)
		argNum++
	}

	if opts.Action != "" {
		query += fmt.Sprintf(" AND action = $%d", argNum)
		args = append(args, opts.Action)
		argNum++
	}

	if !opts.StartTime.IsZero() {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, opts.StartTime)
		argNum++
	}

	if !opts.EndTime.IsZero() {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, opts.EndTime)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	} else {
		query += " LIMIT 100"
	}

	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	var entries []*Entry
	err := l.db.SelectContext(ctx, &entries, query, args...)
	return entries, err
}

// QueryOptions holds options for querying audit logs
type QueryOptions struct {
	TenantID     uuid.UUID
	UserID       *uuid.UUID
	ResourceType string
	ResourceID   *uuid.UUID
	Action       string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
	Offset       int
}

// EntryBuilder provides a fluent interface for building audit entries
type EntryBuilder struct {
	entry *Entry
}

// NewEntry creates a new entry builder
func NewEntry(tenantID uuid.UUID, action, resourceType string) *EntryBuilder {
	return &EntryBuilder{
		entry: &Entry{
			ID:           uuid.New(),
			TenantID:     tenantID,
			Action:       action,
			ResourceType: resourceType,
			CreatedAt:    time.Now(),
			Metadata:     make(map[string]any),
		},
	}
}

// WithUser sets the user ID
func (b *EntryBuilder) WithUser(userID uuid.UUID) *EntryBuilder {
	b.entry.UserID = &userID
	return b
}

// WithResource sets the resource ID
func (b *EntryBuilder) WithResource(resourceID uuid.UUID) *EntryBuilder {
	b.entry.ResourceID = &resourceID
	return b
}

// WithChanges sets the changes
func (b *EntryBuilder) WithChanges(before, after map[string]interface{}) *EntryBuilder {
	changes := Changes{Before: before, After: after}
	data, _ := json.Marshal(changes)
	b.entry.Changes = data
	return b
}

// WithIP sets the IP address
func (b *EntryBuilder) WithIP(ip net.IP) *EntryBuilder {
	if ip != nil {
		s := ip.String()
		b.entry.IPAddress = &s
	}
	return b
}

// WithUserAgent sets the user agent
func (b *EntryBuilder) WithUserAgent(ua string) *EntryBuilder {
	if ua != "" {
		b.entry.UserAgent = &ua
	}
	return b
}

// WithRequestID sets the request ID
func (b *EntryBuilder) WithRequestID(requestID string) *EntryBuilder {
	if requestID != "" {
		b.entry.RequestID = &requestID
	}
	return b
}

// WithAPIKey sets the API key ID
func (b *EntryBuilder) WithAPIKey(apiKeyID uuid.UUID) *EntryBuilder {
	b.entry.APIKeyID = &apiKeyID
	return b
}

// WithMetadata adds metadata
func (b *EntryBuilder) WithMetadata(key string, value any) *EntryBuilder {
	b.entry.Metadata[key] = value
	return b
}

// Build returns the entry
func (b *EntryBuilder) Build() *Entry {
	return b.entry
}
