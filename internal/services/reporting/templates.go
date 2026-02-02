package reporting

// DashboardTemplate is the HTML dashboard template
const DashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Report | {{.Executive.Status | title}}</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        .status-passed { color: #10b981; }
        .status-failed { color: #ef4444; }
        .status-unstable { color: #f59e0b; }
        .bg-status-passed { background-color: #d1fae5; border-color: #10b981; }
        .bg-status-failed { background-color: #fee2e2; border-color: #ef4444; }
        .bg-status-unstable { background-color: #fef3c7; border-color: #f59e0b; }
    </style>
</head>
<body class="bg-gray-50 min-h-screen">
    <!-- Header -->
    <header class="bg-white shadow-sm border-b sticky top-0 z-50">
        <div class="max-w-7xl mx-auto px-4 py-4 flex justify-between items-center">
            <div>
                <h1 class="text-xl font-bold text-gray-900">TestForge Report</h1>
                <p class="text-sm text-gray-500">{{.RunID}} • {{.GeneratedAt.Format "Jan 2, 2006 3:04 PM"}}</p>
            </div>
            <div class="flex space-x-3">
                <button onclick="window.print()" class="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded-lg text-sm">
                    Print / PDF
                </button>
                <button onclick="copyShareLink()" class="px-4 py-2 border hover:bg-gray-50 rounded-lg text-sm">
                    Share
                </button>
            </div>
        </div>
    </header>

    <main class="max-w-7xl mx-auto px-4 py-6 space-y-6">
        <!-- Status Banner -->
        <div class="bg-status-{{.Executive.Status}} border-l-4 rounded-xl p-6">
            <div class="flex items-center justify-between">
                <div>
                    <h2 class="text-2xl font-bold status-{{.Executive.Status}}">
                        {{if eq .Executive.Status "passed"}}✓ All Tests Passed{{else if eq .Executive.Status "failed"}}✗ Tests Failed{{else}}⚠ Unstable{{end}}
                    </h2>
                    <p class="text-gray-700 mt-1">{{.Executive.OneLiner}}</p>
                </div>
                <div class="text-right">
                    <p class="text-4xl font-bold status-{{.Executive.Status}}">{{printf "%.0f" .Executive.HealthScore}}%</p>
                    <p class="text-sm text-gray-600">Health Score</p>
                </div>
            </div>
        </div>

        <!-- Executive Summary Cards -->
        <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
            <!-- Total Tests -->
            <div class="bg-white rounded-xl shadow p-5">
                <p class="text-sm text-gray-500">Total Tests</p>
                <p class="text-2xl font-bold text-gray-900">{{.Executive.TotalTests}}</p>
            </div>

            <!-- Passed -->
            <div class="bg-white rounded-xl shadow p-5 border-l-4 border-green-500">
                <p class="text-sm text-gray-500">Passed</p>
                <p class="text-2xl font-bold text-green-600">{{.Executive.Passed}}</p>
            </div>

            <!-- Failed -->
            <div class="bg-white rounded-xl shadow p-5 border-l-4 border-red-500">
                <p class="text-sm text-gray-500">Failed</p>
                <p class="text-2xl font-bold text-red-600">{{.Executive.Failed}}</p>
            </div>

            <!-- Healed -->
            {{if gt .Executive.Healed 0}}
            <div class="bg-white rounded-xl shadow p-5 border-l-4 border-blue-500">
                <p class="text-sm text-gray-500">Auto-Healed</p>
                <p class="text-2xl font-bold text-blue-600">{{.Executive.Healed}}</p>
            </div>
            {{else}}
            <div class="bg-white rounded-xl shadow p-5 border-l-4 border-gray-300">
                <p class="text-sm text-gray-500">Skipped</p>
                <p class="text-2xl font-bold text-gray-600">{{.Executive.Skipped}}</p>
            </div>
            {{end}}

            <!-- Deployment Status -->
            <div class="bg-white rounded-xl shadow p-5">
                <p class="text-sm text-gray-500">Deployment</p>
                {{if .Executive.DeploymentSafe}}
                <p class="text-xl font-bold text-green-600">✓ Safe</p>
                {{else}}
                <p class="text-xl font-bold text-red-600">✗ Blocked</p>
                {{end}}
                <p class="text-xs text-gray-500 mt-1 truncate">{{.Executive.DeploymentReason}}</p>
            </div>
        </div>

        <!-- AI Insights Banner -->
        {{if .AIInsights}}
        <div class="bg-gradient-to-r from-purple-600 to-blue-600 rounded-xl p-6 text-white">
            <div class="flex items-start space-x-4">
                <div class="text-3xl">🤖</div>
                <div class="flex-1">
                    <h3 class="text-lg font-semibold">AI Analysis</h3>
                    <p class="mt-2 text-white/90">{{.AIInsights.Summary}}</p>
                    {{if .AIInsights.Recommendations}}
                    <div class="mt-4">
                        <p class="text-sm text-white/70 mb-2">Recommendations:</p>
                        <div class="flex flex-wrap gap-2">
                            {{range .AIInsights.Recommendations}}
                            <span class="px-3 py-1 bg-white/20 rounded-full text-sm">{{.Title}}</span>
                            {{end}}
                        </div>
                    </div>
                    {{end}}
                </div>
            </div>
        </div>
        {{end}}

        <!-- Self-Healing Summary -->
        {{if .Healing}}
        {{if gt .Healing.Healed 0}}
        <div class="bg-blue-50 border border-blue-200 rounded-xl p-6">
            <div class="flex items-center space-x-4">
                <span class="text-3xl">🔧</span>
                <div class="flex-1">
                    <h3 class="text-lg font-semibold text-blue-900">Self-Healing Applied</h3>
                    <p class="text-blue-700">{{.Healing.Healed}} of {{.Healing.TotalAttempted}} tests were automatically healed. {{.Healing.TimesSaved}}</p>
                </div>
            </div>
            {{if .Healing.HealingDetails}}
            <div class="mt-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {{range .Healing.HealingDetails}}
                <div class="bg-white rounded-lg p-4 border shadow-sm">
                    <p class="font-medium text-gray-900 truncate">{{.TestName}}</p>
                    <div class="mt-2 text-xs">
                        <p class="text-gray-500">Original:</p>
                        <code class="text-red-600 break-all">{{.OriginalSelector}}</code>
                    </div>
                    <div class="mt-1 text-xs">
                        <p class="text-gray-500">Fixed:</p>
                        <code class="text-green-600 break-all">{{.NewSelector}}</code>
                    </div>
                    <div class="mt-2 flex items-center justify-between text-xs">
                        <span class="text-gray-500">{{.RootCause}}</span>
                        <span class="px-2 py-0.5 bg-green-100 text-green-700 rounded">{{printf "%.0f" (mul .Confidence 100)}}% confidence</span>
                    </div>
                </div>
                {{end}}
            </div>
            {{end}}
        </div>
        {{end}}
        {{end}}

        <!-- Test Results -->
        <div class="bg-white rounded-xl shadow">
            <div class="border-b px-6 py-4 flex justify-between items-center">
                <h2 class="text-lg font-semibold">Test Results</h2>
                <div class="flex items-center space-x-2 text-sm text-gray-500">
                    <span class="inline-flex items-center"><span class="w-2 h-2 bg-green-500 rounded-full mr-1"></span>Passed</span>
                    <span class="inline-flex items-center"><span class="w-2 h-2 bg-red-500 rounded-full mr-1"></span>Failed</span>
                    <span class="inline-flex items-center"><span class="w-2 h-2 bg-gray-300 rounded-full mr-1"></span>Skipped</span>
                </div>
            </div>
            <div class="divide-y">
                {{range .Results.Suites}}
                <div class="p-4">
                    <div class="flex items-center justify-between py-2 px-4 bg-gray-50 rounded-lg cursor-pointer" onclick="toggleSuite(this)">
                        <div class="flex items-center space-x-3">
                            {{if eq .Status "passed"}}
                            <span class="w-3 h-3 rounded-full bg-green-500"></span>
                            {{else}}
                            <span class="w-3 h-3 rounded-full bg-red-500"></span>
                            {{end}}
                            <span class="font-medium">{{.Name}}</span>
                            <span class="text-sm text-gray-500">({{len .Tests}} tests)</span>
                        </div>
                        <div class="flex items-center space-x-4">
                            <span class="text-sm text-green-600">{{.Passed}} ✓</span>
                            {{if gt .Failed 0}}<span class="text-sm text-red-600">{{.Failed}} ✗</span>{{end}}
                            <span class="text-sm text-gray-500">{{.Duration}}</span>
                            <svg class="w-5 h-5 text-gray-400 transform transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path>
                            </svg>
                        </div>
                    </div>
                    <div class="suite-tests hidden ml-4 mt-2 space-y-1">
                        {{range .Tests}}
                        <div class="flex items-center justify-between py-2 px-4 border rounded hover:bg-gray-50">
                            <div class="flex items-center space-x-3">
                                {{if eq .Status "passed"}}
                                <span class="text-green-500">✓</span>
                                {{else if eq .Status "failed"}}
                                <span class="text-red-500">✗</span>
                                {{else}}
                                <span class="text-gray-400">○</span>
                                {{end}}
                                <span class="{{if eq .Status "failed"}}text-red-700{{end}}">{{.Name}}</span>
                                {{if .WasHealed}}
                                <span class="px-2 py-0.5 bg-blue-100 text-blue-700 text-xs rounded">Healed</span>
                                {{end}}
                                {{if gt .RetryCount 0}}
                                <span class="px-2 py-0.5 bg-yellow-100 text-yellow-700 text-xs rounded">Flaky</span>
                                {{end}}
                            </div>
                            <div class="flex items-center space-x-3 text-sm">
                                {{if .ScreenshotURI}}
                                <a href="{{.ScreenshotURI}}" target="_blank" class="text-blue-600 hover:underline">Screenshot</a>
                                {{end}}
                                {{if .VideoURI}}
                                <a href="{{.VideoURI}}" target="_blank" class="text-blue-600 hover:underline">Video</a>
                                {{end}}
                                {{if .TraceURI}}
                                <a href="{{.TraceURI}}" target="_blank" class="text-blue-600 hover:underline">Trace</a>
                                {{end}}
                                <span class="text-gray-500">{{.Duration}}</span>
                            </div>
                        </div>
                        {{end}}
                    </div>
                </div>
                {{end}}
            </div>
        </div>

        <!-- Failed Tests Detail -->
        {{if .Results.FailedTests}}
        <div class="bg-white rounded-xl shadow">
            <div class="border-b px-6 py-4 bg-red-50">
                <h2 class="text-lg font-semibold text-red-900">Failed Tests ({{len .Results.FailedTests}})</h2>
            </div>
            <div class="divide-y">
                {{range .Results.FailedTests}}
                <div class="p-6">
                    <div class="flex justify-between items-start">
                        <div class="flex-1">
                            <h3 class="font-medium text-red-900">{{.Name}}</h3>
                            <p class="text-sm text-gray-500">{{.Suite}}</p>
                            {{if .Error}}
                            <div class="mt-3 p-3 bg-red-50 border border-red-200 rounded-lg">
                                <p class="text-sm text-red-800 font-mono break-words">{{.Error.Message}}</p>
                            </div>
                            {{end}}
                        </div>
                        <div class="flex space-x-2 ml-4">
                            {{if .ScreenshotURI}}
                            <a href="{{.ScreenshotURI}}" target="_blank" class="px-3 py-1 border rounded text-sm hover:bg-gray-50">📷 Screenshot</a>
                            {{end}}
                            {{if .VideoURI}}
                            <a href="{{.VideoURI}}" target="_blank" class="px-3 py-1 border rounded text-sm hover:bg-gray-50">🎬 Video</a>
                            {{end}}
                            {{if .TraceURI}}
                            <a href="{{.TraceURI}}" target="_blank" class="px-3 py-1 border rounded text-sm hover:bg-gray-50">📊 Trace</a>
                            {{end}}
                        </div>
                    </div>

                    {{if .AIAnalysis}}
                    <div class="mt-4 p-4 bg-purple-50 border border-purple-200 rounded-lg">
                        <p class="text-sm font-medium text-purple-900">🤖 AI Analysis</p>
                        <div class="mt-2 text-sm text-purple-800 space-y-1">
                            <p><strong>Root Cause:</strong> {{.AIAnalysis.RootCause}}</p>
                            <p><strong>Suggested Fix:</strong> {{.AIAnalysis.SuggestedFix}}</p>
                        </div>
                    </div>
                    {{end}}

                    {{if .Error}}
                    {{if .Error.Stack}}
                    <details class="mt-4">
                        <summary class="text-sm text-gray-500 cursor-pointer hover:text-gray-700">Show Stack Trace</summary>
                        <pre class="mt-2 p-3 bg-gray-900 text-gray-100 rounded text-xs overflow-x-auto max-h-64">{{.Error.Stack}}</pre>
                    </details>
                    {{end}}
                    {{end}}
                </div>
                {{end}}
            </div>
        </div>
        {{end}}

        <!-- Visual Diffs -->
        {{if .VisualDiffs}}
        <div class="bg-white rounded-xl shadow">
            <div class="border-b px-6 py-4">
                <h2 class="text-lg font-semibold">Visual Differences</h2>
            </div>
            <div class="p-6 grid grid-cols-1 md:grid-cols-2 gap-6">
                {{range .VisualDiffs}}
                <div class="border rounded-lg overflow-hidden">
                    <div class="p-3 bg-gray-50 flex justify-between items-center">
                        <span class="font-medium truncate">{{.TestName}}</span>
                        <span class="px-2 py-1 rounded text-sm {{if .SemanticMatch}}bg-green-100 text-green-800{{else}}bg-red-100 text-red-800{{end}}">
                            {{printf "%.0f" (mul .SimilarityScore 100)}}% match
                        </span>
                    </div>
                    <div class="p-3 grid grid-cols-3 gap-2">
                        <div>
                            <p class="text-xs text-gray-500 mb-1">Baseline</p>
                            <img src="{{.BaselineURI}}" class="rounded border w-full h-24 object-cover" alt="Baseline">
                        </div>
                        <div>
                            <p class="text-xs text-gray-500 mb-1">Actual</p>
                            <img src="{{.ActualURI}}" class="rounded border w-full h-24 object-cover" alt="Actual">
                        </div>
                        <div>
                            <p class="text-xs text-gray-500 mb-1">Diff</p>
                            <img src="{{.DiffURI}}" class="rounded border w-full h-24 object-cover" alt="Diff">
                        </div>
                    </div>
                    <div class="p-3 bg-blue-50 text-sm text-blue-800">{{.Analysis}}</div>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}

        <!-- Compliance & Performance -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <!-- Compliance -->
            <div class="bg-white rounded-xl shadow p-6">
                <h3 class="text-lg font-semibold mb-4">Compliance</h3>
                <div class="flex items-center justify-between mb-4">
                    <span class="text-gray-600">Overall Score</span>
                    <span class="text-2xl font-bold {{if gt .Compliance.OverallScore 80.0}}text-green-600{{else if gt .Compliance.OverallScore 60.0}}text-yellow-600{{else}}text-red-600{{end}}">
                        {{printf "%.0f" .Compliance.OverallScore}}%
                    </span>
                </div>
                <div class="space-y-2">
                    {{range .Compliance.Standards}}
                    <div class="flex items-center justify-between text-sm">
                        <span>{{.Name}}</span>
                        <span class="{{if eq .Status "completed"}}text-green-600{{else}}text-yellow-600{{end}}">{{.Passed}}/{{.Controls}}</span>
                    </div>
                    {{end}}
                </div>
            </div>

            <!-- Performance -->
            <div class="bg-white rounded-xl shadow p-6">
                <h3 class="text-lg font-semibold mb-4">Web Vitals</h3>
                <div class="space-y-3">
                    <div class="flex items-center justify-between">
                        <span class="text-gray-600">LCP</span>
                        <span class="px-2 py-1 rounded text-sm {{if eq .Performance.WebVitals.LCP.Rating "good"}}bg-green-100 text-green-800{{else if eq .Performance.WebVitals.LCP.Rating "needs-improvement"}}bg-yellow-100 text-yellow-800{{else}}bg-red-100 text-red-800{{end}}">
                            {{.Performance.WebVitals.LCP.Value}}{{.Performance.WebVitals.LCP.Unit}}
                        </span>
                    </div>
                    <div class="flex items-center justify-between">
                        <span class="text-gray-600">FID</span>
                        <span class="px-2 py-1 rounded text-sm {{if eq .Performance.WebVitals.FID.Rating "good"}}bg-green-100 text-green-800{{else if eq .Performance.WebVitals.FID.Rating "needs-improvement"}}bg-yellow-100 text-yellow-800{{else}}bg-red-100 text-red-800{{end}}">
                            {{.Performance.WebVitals.FID.Value}}{{.Performance.WebVitals.FID.Unit}}
                        </span>
                    </div>
                    <div class="flex items-center justify-between">
                        <span class="text-gray-600">CLS</span>
                        <span class="px-2 py-1 rounded text-sm {{if eq .Performance.WebVitals.CLS.Rating "good"}}bg-green-100 text-green-800{{else if eq .Performance.WebVitals.CLS.Rating "needs-improvement"}}bg-yellow-100 text-yellow-800{{else}}bg-red-100 text-red-800{{end}}">
                            {{.Performance.WebVitals.CLS.Value}}
                        </span>
                    </div>
                </div>
            </div>
        </div>

        <!-- Audit Trail -->
        <div class="bg-white rounded-xl shadow">
            <div class="border-b px-6 py-4">
                <h2 class="text-lg font-semibold">Audit Trail</h2>
            </div>
            <div class="p-6">
                <div class="space-y-3">
                    {{range .AuditTrail}}
                    <div class="flex items-start space-x-3 text-sm">
                        <span class="text-gray-400 w-40 flex-shrink-0">{{.Timestamp.Format "15:04:05"}}</span>
                        <span class="text-gray-600">{{.Action}}</span>
                        <span class="text-gray-500">{{.Details}}</span>
                    </div>
                    {{end}}
                </div>
            </div>
        </div>
    </main>

    <!-- Footer -->
    <footer class="max-w-7xl mx-auto px-4 py-6 text-center text-sm text-gray-500 border-t mt-8">
        Generated by <strong>TestForge</strong> • {{.GeneratedAt.Format "January 2, 2006 at 3:04 PM"}} • Report ID: {{.ID}}
    </footer>

    <script>
        function toggleSuite(element) {
            const tests = element.nextElementSibling;
            const arrow = element.querySelector('svg');
            tests.classList.toggle('hidden');
            arrow.classList.toggle('rotate-180');
        }

        function copyShareLink() {
            const url = window.location.href;
            navigator.clipboard.writeText(url).then(() => {
                alert('Report link copied to clipboard!');
            });
        }

        // Auto-expand failed suites
        document.addEventListener('DOMContentLoaded', () => {
            document.querySelectorAll('.suite-tests').forEach(suite => {
                if (suite.querySelector('.text-red-500')) {
                    suite.classList.remove('hidden');
                    suite.previousElementSibling.querySelector('svg').classList.add('rotate-180');
                }
            });
        });
    </script>
</body>
</html>`
