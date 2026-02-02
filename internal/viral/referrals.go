package viral

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ReferralRewardType represents the type of referral reward
type ReferralRewardType string

const (
	RewardTypeFreeMonth ReferralRewardType = "free_month"
	RewardTypeDiscount  ReferralRewardType = "discount"
	RewardTypeCredits   ReferralRewardType = "credits"
)

// Referral represents a referral code
type Referral struct {
	ID               uuid.UUID          `db:"id" json:"id"`
	ReferrerTenantID uuid.UUID          `db:"referrer_tenant_id" json:"referrer_tenant_id"`
	Code             string             `db:"code" json:"code"`
	Clicks           int                `db:"clicks" json:"clicks"`
	Signups          int                `db:"signups" json:"signups"`
	Conversions      int                `db:"conversions" json:"conversions"`
	RewardType       ReferralRewardType `db:"reward_type" json:"reward_type"`
	RewardClaimed    bool               `db:"reward_claimed" json:"reward_claimed"`
	ExpiresAt        *time.Time         `db:"expires_at" json:"expires_at,omitempty"`
	CreatedAt        time.Time          `db:"created_at" json:"created_at"`
}

// ReferredUser represents a user who signed up via referral
type ReferredUser struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	ReferralID       uuid.UUID  `db:"referral_id" json:"referral_id"`
	ReferredTenantID uuid.UUID  `db:"referred_tenant_id" json:"referred_tenant_id"`
	SignedUpAt       time.Time  `db:"signed_up_at" json:"signed_up_at"`
	ConvertedAt      *time.Time `db:"converted_at" json:"converted_at,omitempty"`
}

// ReferralService manages the referral program
type ReferralService struct {
	db      *sqlx.DB
	logger  *zap.Logger
	baseURL string
}

// NewReferralService creates a new referral service
func NewReferralService(db *sqlx.DB, baseURL string, logger *zap.Logger) *ReferralService {
	return &ReferralService{
		db:      db,
		logger:  logger,
		baseURL: baseURL,
	}
}

// CreateReferralCode creates a new referral code for a tenant
func (rs *ReferralService) CreateReferralCode(ctx context.Context, tenantID uuid.UUID, rewardType ReferralRewardType, expiresAt *time.Time) (*Referral, error) {
	code, err := generateReferralCode()
	if err != nil {
		return nil, fmt.Errorf("generating code: %w", err)
	}

	referral := &Referral{
		ID:               uuid.New(),
		ReferrerTenantID: tenantID,
		Code:             code,
		RewardType:       rewardType,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
	}

	_, err = rs.db.ExecContext(ctx, `
		INSERT INTO referrals (id, referrer_tenant_id, code, reward_type, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		referral.ID, referral.ReferrerTenantID, referral.Code, referral.RewardType, referral.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting referral: %w", err)
	}

	return referral, nil
}

// GetReferralByCode retrieves a referral by its code
func (rs *ReferralService) GetReferralByCode(ctx context.Context, code string) (*Referral, error) {
	var referral Referral
	err := rs.db.GetContext(ctx, &referral, `
		SELECT * FROM referrals WHERE code = $1`,
		strings.ToUpper(code),
	)
	if err != nil {
		return nil, fmt.Errorf("getting referral: %w", err)
	}

	return &referral, nil
}

// GetReferralsForTenant gets all referral codes for a tenant
func (rs *ReferralService) GetReferralsForTenant(ctx context.Context, tenantID uuid.UUID) ([]Referral, error) {
	var referrals []Referral
	err := rs.db.SelectContext(ctx, &referrals, `
		SELECT * FROM referrals
		WHERE referrer_tenant_id = $1
		ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing referrals: %w", err)
	}

	return referrals, nil
}

// RecordClick records a click on a referral link
func (rs *ReferralService) RecordClick(ctx context.Context, code string) error {
	result, err := rs.db.ExecContext(ctx, `
		UPDATE referrals
		SET clicks = clicks + 1
		WHERE code = $1
		  AND (expires_at IS NULL OR expires_at > NOW())`,
		strings.ToUpper(code),
	)
	if err != nil {
		return fmt.Errorf("recording click: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("referral code not found or expired")
	}

	return nil
}

// RecordSignup records a signup from a referral
func (rs *ReferralService) RecordSignup(ctx context.Context, code string, referredTenantID uuid.UUID) error {
	// Get referral
	referral, err := rs.GetReferralByCode(ctx, code)
	if err != nil {
		return err
	}

	// Check if already referred
	var exists bool
	err = rs.db.GetContext(ctx, &exists, `
		SELECT EXISTS(SELECT 1 FROM referred_users WHERE referred_tenant_id = $1)`,
		referredTenantID,
	)
	if err != nil {
		return fmt.Errorf("checking existing referral: %w", err)
	}
	if exists {
		return fmt.Errorf("tenant already referred")
	}

	// Check expiration
	if referral.ExpiresAt != nil && time.Now().After(*referral.ExpiresAt) {
		return fmt.Errorf("referral code expired")
	}

	// Record signup
	tx, err := rs.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO referred_users (id, referral_id, referred_tenant_id)
		VALUES ($1, $2, $3)`,
		uuid.New(), referral.ID, referredTenantID,
	)
	if err != nil {
		return fmt.Errorf("inserting referred user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE referrals
		SET signups = signups + 1
		WHERE id = $1`,
		referral.ID,
	)
	if err != nil {
		return fmt.Errorf("updating signup count: %w", err)
	}

	return tx.Commit()
}

// RecordConversion records a paid conversion from a referral
func (rs *ReferralService) RecordConversion(ctx context.Context, referredTenantID uuid.UUID) error {
	tx, err := rs.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update referred user
	result, err := tx.ExecContext(ctx, `
		UPDATE referred_users
		SET converted_at = NOW()
		WHERE referred_tenant_id = $1 AND converted_at IS NULL`,
		referredTenantID,
	)
	if err != nil {
		return fmt.Errorf("updating referred user: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil // Not a referred user or already converted
	}

	// Get referral ID and update conversion count
	var referralID uuid.UUID
	err = tx.GetContext(ctx, &referralID, `
		SELECT referral_id FROM referred_users WHERE referred_tenant_id = $1`,
		referredTenantID,
	)
	if err != nil {
		return fmt.Errorf("getting referral ID: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE referrals
		SET conversions = conversions + 1
		WHERE id = $1`,
		referralID,
	)
	if err != nil {
		return fmt.Errorf("updating conversion count: %w", err)
	}

	return tx.Commit()
}

// ClaimReward claims the reward for a referral
func (rs *ReferralService) ClaimReward(ctx context.Context, referralID uuid.UUID, tenantID uuid.UUID) error {
	// Verify ownership and eligibility
	var referral Referral
	err := rs.db.GetContext(ctx, &referral, `
		SELECT * FROM referrals
		WHERE id = $1 AND referrer_tenant_id = $2`,
		referralID, tenantID,
	)
	if err != nil {
		return fmt.Errorf("referral not found or unauthorized")
	}

	if referral.RewardClaimed {
		return fmt.Errorf("reward already claimed")
	}

	// Check minimum conversions for reward
	minConversions := getMinConversions(referral.RewardType)
	if referral.Conversions < minConversions {
		return fmt.Errorf("need %d conversions to claim reward, have %d", minConversions, referral.Conversions)
	}

	// Mark as claimed (actual reward application happens in billing)
	_, err = rs.db.ExecContext(ctx, `
		UPDATE referrals
		SET reward_claimed = true
		WHERE id = $1`,
		referralID,
	)
	if err != nil {
		return fmt.Errorf("claiming reward: %w", err)
	}

	rs.logger.Info("referral reward claimed",
		zap.String("referral_id", referralID.String()),
		zap.String("tenant_id", tenantID.String()),
		zap.String("reward_type", string(referral.RewardType)),
	)

	return nil
}

// GetReferralURL returns the referral URL for a code
func (rs *ReferralService) GetReferralURL(code string) string {
	return fmt.Sprintf("%s/signup?ref=%s", rs.baseURL, code)
}

// GetReferralStats returns statistics for a tenant's referral program
func (rs *ReferralService) GetReferralStats(ctx context.Context, tenantID uuid.UUID) (*ReferralStats, error) {
	var stats ReferralStats

	err := rs.db.GetContext(ctx, &stats, `
		SELECT
			COALESCE(SUM(clicks), 0) as total_clicks,
			COALESCE(SUM(signups), 0) as total_signups,
			COALESCE(SUM(conversions), 0) as total_conversions,
			COUNT(*) as total_codes,
			COUNT(*) FILTER (WHERE reward_claimed = true) as rewards_claimed
		FROM referrals
		WHERE referrer_tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting stats: %w", err)
	}

	// Calculate conversion rates
	if stats.TotalClicks > 0 {
		stats.ClickToSignupRate = float64(stats.TotalSignups) / float64(stats.TotalClicks)
	}
	if stats.TotalSignups > 0 {
		stats.SignupToConversionRate = float64(stats.TotalConversions) / float64(stats.TotalSignups)
	}

	return &stats, nil
}

// ReferralStats contains referral program statistics
type ReferralStats struct {
	TotalClicks            int     `db:"total_clicks" json:"total_clicks"`
	TotalSignups           int     `db:"total_signups" json:"total_signups"`
	TotalConversions       int     `db:"total_conversions" json:"total_conversions"`
	TotalCodes             int     `db:"total_codes" json:"total_codes"`
	RewardsClaimed         int     `db:"rewards_claimed" json:"rewards_claimed"`
	ClickToSignupRate      float64 `json:"click_to_signup_rate"`
	SignupToConversionRate float64 `json:"signup_to_conversion_rate"`
}

// Helper functions

func generateReferralCode() (string, error) {
	bytes := make([]byte, 5)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return strings.ToUpper(base32.StdEncoding.EncodeToString(bytes)[:8]), nil
}

func getMinConversions(rewardType ReferralRewardType) int {
	switch rewardType {
	case RewardTypeFreeMonth:
		return 3
	case RewardTypeDiscount:
		return 1
	case RewardTypeCredits:
		return 1
	default:
		return 1
	}
}
