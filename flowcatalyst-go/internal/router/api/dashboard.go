package api

// dashboardHTML contains the monitoring dashboard HTML page
// This matches the Java implementation at /monitoring/dashboard
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FlowCatalyst Dashboard</title>
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='6' fill='%2347a3f3'/%3E%3Cpath d='M17.5 13V6L8 17h6.5v7L24 13h-6.5z' fill='white' stroke='white' stroke-width='0.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E">
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
</head>
<body class="bg-gray-100 min-h-screen">
    <div class="container mx-auto px-4 py-8">
        <!-- Header -->
        <div class="mb-8">
            <div class="flex justify-between items-start mb-4">
                <h1 class="text-3xl font-bold text-gray-900">Flow Catalyst Dashboard</h1>
            </div>
            <div class="flex items-center space-x-4">
                <div id="statusContainer" class="flex items-center cursor-pointer hover:opacity-70">
                    <div id="statusIndicator" class="w-3 h-3 rounded-full mr-2"></div>
                    <span id="statusText" class="text-sm font-medium">Loading...</span>
                </div>
                <span id="uptimeText" class="text-sm text-gray-600"></span>
                <button id="refreshBtn" class="bg-blue-500 hover:bg-blue-600 text-white px-4 py-2 rounded text-sm">
                    Refresh
                </button>
            </div>
        </div>

        <!-- Health Status Modal -->
        <div id="healthModal" class="hidden fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center">
            <div class="bg-white rounded-lg shadow-lg max-w-md w-full mx-4">
                <div class="px-6 py-4 border-b border-gray-200">
                    <h3 class="text-lg font-semibold text-gray-900">System Status Details</h3>
                </div>
                <div class="px-6 py-4">
                    <div id="modalContent" class="space-y-3">
                    </div>
                </div>
                <div class="px-6 py-4 border-t border-gray-200 flex justify-end">
                    <button onclick="document.getElementById('healthModal').classList.add('hidden')" class="bg-blue-500 hover:bg-blue-600 text-white px-4 py-2 rounded text-sm">
                        Close
                    </button>
                </div>
            </div>
        </div>

        <!-- Key Metrics Cards -->
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
            <div class="bg-white rounded-lg shadow p-6">
                <div class="flex items-center">
                    <div class="p-2 bg-blue-100 rounded-lg">
                        <svg class="w-6 h-6 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"></path>
                        </svg>
                    </div>
                    <div class="ml-4">
                        <p class="text-sm font-medium text-gray-600">Active Queues</p>
                        <p id="activeQueues" class="text-2xl font-semibold text-gray-900">-</p>
                    </div>
                </div>
            </div>

            <div class="bg-white rounded-lg shadow p-6">
                <div class="flex items-center">
                    <div class="p-2 bg-green-100 rounded-lg">
                        <svg class="w-6 h-6 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
                        </svg>
                    </div>
                    <div class="ml-4">
                        <p class="text-sm font-medium text-gray-600">Total Processed</p>
                        <p id="totalProcessed" class="text-2xl font-semibold text-gray-900">-</p>
                    </div>
                </div>
            </div>

            <div class="bg-white rounded-lg shadow p-6">
                <div class="flex items-center">
                    <div class="p-2 bg-yellow-100 rounded-lg">
                        <svg class="w-6 h-6 text-yellow-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z"></path>
                        </svg>
                    </div>
                    <div class="ml-4">
                        <p class="text-sm font-medium text-gray-600">Active Warnings</p>
                        <p id="activeWarnings" class="text-2xl font-semibold text-gray-900">-</p>
                    </div>
                </div>
            </div>

            <div class="bg-white rounded-lg shadow p-6">
                <div class="flex items-center">
                    <div class="p-2 bg-red-100 rounded-lg">
                        <svg class="w-6 h-6 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                        </svg>
                    </div>
                    <div class="ml-4">
                        <p class="text-sm font-medium text-gray-600">Circuit Breakers Open</p>
                        <p id="circuitBreakersOpen" class="text-2xl font-semibold text-gray-900">-</p>
                    </div>
                </div>
            </div>
        </div>

        <!-- Tabbed Content Section -->
        <div class="bg-white rounded-lg shadow">
            <!-- Tab Navigation -->
            <div class="border-b border-gray-200">
                <nav class="flex -mb-px">
                    <button id="tabQueues" class="tab-button active px-6 py-4 text-sm font-medium border-b-2 border-blue-500 text-blue-600">
                        Queue Statistics
                    </button>
                    <button id="tabPools" class="tab-button px-6 py-4 text-sm font-medium border-b-2 border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300">
                        Pool Statistics
                    </button>
                    <button id="tabWarnings" class="tab-button px-6 py-4 text-sm font-medium border-b-2 border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300">
                        Warnings
                    </button>
                    <button id="tabInFlight" class="tab-button px-6 py-4 text-sm font-medium border-b-2 border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300">
                        In-Flight Messages
                    </button>
                </nav>
            </div>

            <!-- Tab Content -->
            <div id="tabContent">
                <!-- Queue Statistics Tab -->
                <div id="contentQueues" class="tab-content">
                    <div class="px-6 py-4 border-b border-gray-200">
                        <h3 class="text-lg font-semibold text-gray-900">Queue Statistics</h3>
                        <p class="text-sm text-gray-600">Messages processed per queue</p>
                    </div>
                    <!-- Queue Success Rate Chart -->
                    <div class="px-6 py-4 border-b border-gray-200 bg-gray-50">
                        <h4 class="text-base font-semibold text-gray-900 mb-4">Queue Success Rate</h4>
                        <div class="h-64">
                            <canvas id="queueSuccessChart"></canvas>
                        </div>
                    </div>
                    <div class="overflow-x-auto">
                        <table class="min-w-full divide-y divide-gray-200">
                            <thead class="bg-gray-50">
                                <tr>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Queue</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Pending</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">In-Flight</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Total</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Success</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Failed</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Success Rate</th>
                                </tr>
                            </thead>
                            <tbody id="queueStatsTable" class="bg-white divide-y divide-gray-200">
                            </tbody>
                        </table>
                    </div>
                </div>

                <!-- Pool Statistics Tab -->
                <div id="contentPools" class="tab-content hidden">
                    <div class="px-6 py-4 border-b border-gray-200">
                        <h3 class="text-lg font-semibold text-gray-900">Pool Statistics</h3>
                        <p class="text-sm text-gray-600">Message processing pools</p>
                    </div>
                    <!-- Pool Success Rate Chart -->
                    <div class="px-6 py-4 border-b border-gray-200 bg-gray-50">
                        <h4 class="text-base font-semibold text-gray-900 mb-4">Pool Success Rate</h4>
                        <div class="h-64">
                            <canvas id="poolSuccessChart"></canvas>
                        </div>
                    </div>
                    <div class="overflow-x-auto">
                        <table class="min-w-full divide-y divide-gray-200">
                            <thead class="bg-gray-50">
                                <tr>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Pool</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Workers</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Queue</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Rate Limited</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Total</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Success</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Success Rate</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Avg Time</th>
                                </tr>
                            </thead>
                            <tbody id="poolStatsTable" class="bg-white divide-y divide-gray-200">
                            </tbody>
                        </table>
                    </div>
                </div>

                <!-- Warnings Tab -->
                <div id="contentWarnings" class="tab-content hidden">
                    <div class="px-6 py-4 border-b border-gray-200">
                        <div class="flex justify-between items-center">
                            <h3 class="text-lg font-semibold text-gray-900">Active Warnings</h3>
                            <div class="flex space-x-4">
                                <select id="severityFilter" class="border border-gray-300 rounded px-3 py-2 text-sm">
                                    <option value="">All Severities</option>
                                    <option value="CRITICAL">CRITICAL</option>
                                    <option value="ERROR">ERROR</option>
                                    <option value="WARN">WARN</option>
                                    <option value="INFO">INFO</option>
                                </select>
                                <input type="text" id="searchFilter" placeholder="Search warnings..."
                                       class="border border-gray-300 rounded px-3 py-2 text-sm w-64">
                            </div>
                        </div>
                    </div>
                    <div class="overflow-x-auto">
                        <table class="min-w-full divide-y divide-gray-200">
                            <thead class="bg-gray-50">
                                <tr>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Time</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Severity</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Category</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Source</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Message</th>
                                </tr>
                            </thead>
                            <tbody id="warningsTableBody" class="bg-white divide-y divide-gray-200">
                            </tbody>
                        </table>
                    </div>
                    <div id="noWarnings" class="hidden px-6 py-8 text-center text-gray-500">
                        No active warnings
                    </div>
                </div>

                <!-- In-Flight Messages Tab -->
                <div id="contentInFlight" class="tab-content hidden">
                    <div class="px-6 py-4 border-b border-gray-200">
                        <div class="flex justify-between items-center">
                            <h3 class="text-lg font-semibold text-gray-900">Messages Currently In-Flight</h3>
                            <div class="flex space-x-4">
                                <input type="text" id="messageIdFilter" placeholder="Search by message ID..."
                                       class="border border-gray-300 rounded px-3 py-2 text-sm w-64">
                                <button id="refreshInFlightBtn" class="bg-blue-600 text-white px-4 py-2 rounded text-sm hover:bg-blue-700">
                                    Refresh
                                </button>
                            </div>
                        </div>
                    </div>
                    <div class="overflow-x-auto">
                        <table class="min-w-full divide-y divide-gray-200">
                            <thead class="bg-gray-50">
                                <tr>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Message ID</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Broker ID</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Queue</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Pool</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Elapsed Time</th>
                                    <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Added At</th>
                                </tr>
                            </thead>
                            <tbody id="inFlightTable" class="bg-white divide-y divide-gray-200">
                            </tbody>
                        </table>
                    </div>
                    <div id="noInFlight" class="hidden px-6 py-8 text-center text-gray-500">
                        No messages in flight
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script>
        class Dashboard {
            constructor() {
                this.charts = {};
                this.data = {
                    health: null,
                    queueStats: {},
                    poolStats: {},
                    warnings: [],
                    circuitBreakers: {}
                };
                this.filters = {
                    severity: '',
                    search: ''
                };
                this.activeTab = 'Queues';

                this.init();
                this.setupEventListeners();
                this.setupTabListeners();
                this.startPeriodicRefresh();
            }

            async init() {
                this.initCharts();
                await this.loadData();
            }

            initCharts() {
                // Queue Success Rate Chart
                const queueSuccessCtx = document.getElementById('queueSuccessChart').getContext('2d');
                this.charts.queueSuccess = new Chart(queueSuccessCtx, {
                    type: 'bar',
                    data: {
                        labels: [],
                        datasets: [{
                            label: 'Success Rate (%)',
                            data: [],
                            backgroundColor: 'rgba(16, 185, 129, 0.8)',
                            borderColor: 'rgba(16, 185, 129, 1)',
                            borderWidth: 1
                        }]
                    },
                    options: {
                        indexAxis: 'y',
                        responsive: true,
                        maintainAspectRatio: false,
                        scales: {
                            x: {
                                beginAtZero: true,
                                max: 100,
                                title: { display: true, text: 'Success Rate (%)' }
                            }
                        }
                    }
                });

                // Pool Success Rate Chart
                const poolSuccessCtx = document.getElementById('poolSuccessChart').getContext('2d');
                this.charts.poolSuccess = new Chart(poolSuccessCtx, {
                    type: 'bar',
                    data: {
                        labels: [],
                        datasets: [{
                            label: 'Success Rate (%)',
                            data: [],
                            backgroundColor: 'rgba(59, 130, 246, 0.8)',
                            borderColor: 'rgba(59, 130, 246, 1)',
                            borderWidth: 1
                        }]
                    },
                    options: {
                        indexAxis: 'y',
                        responsive: true,
                        maintainAspectRatio: false,
                        scales: {
                            x: {
                                beginAtZero: true,
                                max: 100,
                                title: { display: true, text: 'Success Rate (%)' }
                            }
                        }
                    }
                });
            }

            setupTabListeners() {
                const tabs = ['Queues', 'Pools', 'Warnings', 'InFlight'];
                tabs.forEach(tab => {
                    document.getElementById('tab' + tab).addEventListener('click', () => {
                        this.switchTab(tab);
                        if (tab === 'InFlight') {
                            this.loadInFlightMessages();
                        }
                    });
                });

                document.getElementById('refreshInFlightBtn').addEventListener('click', () => {
                    this.loadInFlightMessages();
                });

                document.getElementById('messageIdFilter').addEventListener('input', (e) => {
                    this.filterInFlightMessages(e.target.value);
                });
            }

            switchTab(tabName) {
                this.activeTab = tabName;

                document.querySelectorAll('.tab-button').forEach(btn => {
                    btn.classList.remove('border-blue-500', 'text-blue-600');
                    btn.classList.add('border-transparent', 'text-gray-500');
                });

                const activeBtn = document.getElementById('tab' + tabName);
                activeBtn.classList.remove('border-transparent', 'text-gray-500');
                activeBtn.classList.add('border-blue-500', 'text-blue-600');

                document.querySelectorAll('.tab-content').forEach(content => {
                    content.classList.add('hidden');
                });

                document.getElementById('content' + tabName).classList.remove('hidden');
            }

            setupEventListeners() {
                document.getElementById('statusContainer').addEventListener('click', () => {
                    this.showHealthModal();
                });

                document.getElementById('refreshBtn').addEventListener('click', () => {
                    this.loadData();
                });

                document.getElementById('severityFilter').addEventListener('change', (e) => {
                    this.filters.severity = e.target.value;
                    this.filterWarnings();
                });

                document.getElementById('searchFilter').addEventListener('input', (e) => {
                    this.filters.search = e.target.value.toLowerCase();
                    this.filterWarnings();
                });
            }

            async loadData() {
                try {
                    const responses = await Promise.all([
                        fetch('/monitoring/health'),
                        fetch('/monitoring/queue-stats'),
                        fetch('/monitoring/pool-stats'),
                        fetch('/monitoring/warnings'),
                        fetch('/monitoring/circuit-breakers')
                    ]);

                    const [health, queueStats, poolStats, warnings, circuitBreakers] =
                        await Promise.all(responses.map(r => r.json()));

                    this.data = { health, queueStats, poolStats, warnings, circuitBreakers };
                    this.updateUI();
                } catch (error) {
                    console.error('Failed to load data:', error);
                    this.showError();
                }
            }

            updateUI() {
                this.updateStatus();
                this.updateMetrics();
                this.updateCharts();
                this.updateStatsTables();
                this.updateWarningsTable();
            }

            updateStatus() {
                const status = this.data.health.status;
                const indicator = document.getElementById('statusIndicator');
                const text = document.getElementById('statusText');
                const uptimeText = document.getElementById('uptimeText');

                const statusConfig = {
                    'HEALTHY': { color: 'bg-green-500', text: 'System Healthy' },
                    'WARNING': { color: 'bg-yellow-500', text: 'System Warning' },
                    'DEGRADED': { color: 'bg-red-500', text: 'System Degraded' }
                };

                const config = statusConfig[status] || { color: 'bg-gray-500', text: 'Unknown' };
                indicator.className = 'w-3 h-3 rounded-full mr-2 ' + config.color;
                text.textContent = config.text;

                const uptimeMs = this.data.health.uptimeMillis || 0;
                const uptimeSeconds = Math.floor(uptimeMs / 1000);
                const hours = Math.floor(uptimeSeconds / 3600);
                const minutes = Math.floor((uptimeSeconds % 3600) / 60);
                const seconds = uptimeSeconds % 60;
                uptimeText.textContent = 'Uptime: ' + hours + 'h ' + minutes + 'm ' + seconds + 's';
            }

            updateMetrics() {
                const queueCount = Object.keys(this.data.queueStats).length;
                document.getElementById('activeQueues').textContent = queueCount;

                const totalProcessed = Object.values(this.data.queueStats)
                    .reduce((sum, stats) => sum + (stats.totalMessages || 0), 0);
                document.getElementById('totalProcessed').textContent = totalProcessed.toLocaleString();

                document.getElementById('activeWarnings').textContent = this.data.warnings.length;

                const openCircuitBreakers = Object.values(this.data.circuitBreakers)
                    .filter(cb => cb.state === 'OPEN').length;
                document.getElementById('circuitBreakersOpen').textContent = openCircuitBreakers;
            }

            extractQueueName(queueUrl) {
                const parts = queueUrl.split('/');
                return parts[parts.length - 1];
            }

            updateCharts() {
                // Queue Success Rate Chart
                const queueEntries = Object.entries(this.data.queueStats);
                const queueNames = queueEntries.map(([name, stats]) => this.extractQueueName(stats.name || name));
                const queueSuccessRates = queueEntries.map(([name, stats]) => (stats.successRate || 0) * 100);

                this.charts.queueSuccess.data.labels = queueNames;
                this.charts.queueSuccess.data.datasets[0].data = queueSuccessRates;
                this.charts.queueSuccess.update();

                // Pool Success Rate Chart
                const poolEntries = Object.entries(this.data.poolStats);
                const poolNames = poolEntries.map(([name, stats]) => stats.poolCode || name);
                const poolSuccessRates = poolEntries.map(([name, stats]) => (stats.successRate || 0) * 100);

                this.charts.poolSuccess.data.labels = poolNames;
                this.charts.poolSuccess.data.datasets[0].data = poolSuccessRates;
                this.charts.poolSuccess.update();
            }

            updateStatsTables() {
                this.updateQueueStatsTable();
                this.updatePoolStatsTable();
            }

            updateQueueStatsTable() {
                const tbody = document.getElementById('queueStatsTable');
                const queueEntries = Object.entries(this.data.queueStats);

                if (queueEntries.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="7" class="text-center py-4 text-gray-500">No queue data available</td></tr>';
                    return;
                }

                tbody.innerHTML = queueEntries.map(([queueName, stats]) => {
                    const rate = (stats.successRate || 0) * 100;
                    const rateClass = rate >= 90 ? 'bg-green-100 text-green-800' :
                                     rate >= 75 ? 'bg-yellow-100 text-yellow-800' : 'bg-red-100 text-red-800';
                    return '<tr>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">' + this.extractQueueName(stats.name || queueName) + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-blue-600">' + (stats.pendingMessages || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-orange-600">' + (stats.messagesNotVisible || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">' + (stats.totalMessages || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-green-600">' + (stats.totalConsumed || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-red-600">' + (stats.totalFailed || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900"><span class="px-2 py-1 ' + rateClass + ' rounded text-xs font-medium">' + rate.toFixed(1) + '%</span></td>' +
                    '</tr>';
                }).join('');
            }

            updatePoolStatsTable() {
                const tbody = document.getElementById('poolStatsTable');
                const poolEntries = Object.entries(this.data.poolStats);

                if (poolEntries.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="8" class="text-center py-4 text-gray-500">No pool data available</td></tr>';
                    return;
                }

                tbody.innerHTML = poolEntries.map(([poolCode, stats]) => {
                    const rate = (stats.successRate || 0) * 100;
                    const rateClass = rate >= 90 ? 'bg-green-100 text-green-800' :
                                     rate >= 75 ? 'bg-yellow-100 text-yellow-800' : 'bg-red-100 text-red-800';
                    return '<tr>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">' + (stats.poolCode || poolCode) + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">' + (stats.activeWorkers || 0) + '/' + (stats.maxConcurrency || 0) + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-purple-600">' + (stats.queueSize || 0) + '/' + (stats.maxQueueCapacity || 0) + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-orange-600">' + (stats.totalRateLimited || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">' + (stats.totalProcessed || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-green-600">' + (stats.totalSucceeded || 0).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900"><span class="px-2 py-1 ' + rateClass + ' rounded text-xs font-medium">' + rate.toFixed(1) + '%</span></td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">' + (stats.averageProcessingTimeMs || 0).toFixed(0) + 'ms</td>' +
                    '</tr>';
                }).join('');
            }

            updateWarningsTable() {
                const tbody = document.getElementById('warningsTableBody');
                const noWarnings = document.getElementById('noWarnings');

                if (this.data.warnings.length === 0) {
                    tbody.innerHTML = '';
                    noWarnings.classList.remove('hidden');
                    return;
                }

                noWarnings.classList.add('hidden');
                this.filterWarnings();
            }

            renderWarnings(warnings) {
                const tbody = document.getElementById('warningsTableBody');
                tbody.innerHTML = warnings.map(warning => {
                    const severityColor = warning.severity === 'ERROR' ? 'bg-red-100 text-red-800' :
                                         warning.severity === 'WARN' ? 'bg-yellow-100 text-yellow-800' :
                                         'bg-blue-100 text-blue-800';
                    return '<tr>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">' + new Date(warning.timestamp).toLocaleString() + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900"><span class="px-2 py-1 ' + severityColor + ' rounded text-xs font-medium">' + warning.severity + '</span></td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">' + warning.category + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 max-w-xs truncate">' + warning.source + '</td>' +
                        '<td class="px-6 py-4 text-sm text-gray-900">' + warning.message + '</td>' +
                    '</tr>';
                }).join('');
            }

            filterWarnings() {
                let filtered = this.data.warnings;

                if (this.filters.severity) {
                    filtered = filtered.filter(w => w.severity === this.filters.severity);
                }

                if (this.filters.search) {
                    filtered = filtered.filter(w =>
                        w.message.toLowerCase().includes(this.filters.search) ||
                        w.category.toLowerCase().includes(this.filters.search) ||
                        w.source.toLowerCase().includes(this.filters.search)
                    );
                }

                this.renderWarnings(filtered);
            }

            showError() {
                const indicator = document.getElementById('statusIndicator');
                const text = document.getElementById('statusText');
                indicator.className = 'w-3 h-3 rounded-full mr-2 bg-red-500';
                text.textContent = 'Connection Error';
            }

            async loadInFlightMessages() {
                try {
                    const response = await fetch('/monitoring/in-flight-messages?limit=100');
                    if (!response.ok) {
                        throw new Error('Failed to load in-flight messages');
                    }
                    const inFlightMessages = await response.json();
                    this.data.inFlightMessages = inFlightMessages;
                    this.renderInFlightMessages(inFlightMessages);
                } catch (error) {
                    console.error('Failed to load in-flight messages:', error);
                    document.getElementById('inFlightTable').innerHTML = '';
                    document.getElementById('noInFlight').classList.remove('hidden');
                }
            }

            renderInFlightMessages(messages) {
                const table = document.getElementById('inFlightTable');
                const noMessages = document.getElementById('noInFlight');

                if (!messages || messages.length === 0) {
                    table.innerHTML = '';
                    noMessages.classList.remove('hidden');
                    return;
                }

                noMessages.classList.add('hidden');
                table.innerHTML = messages.map(msg => {
                    const addedAt = new Date(msg.addedToInPipelineAt);
                    const elapsedSec = Math.floor(msg.elapsedTimeMs / 1000);
                    const elapsedMin = Math.floor(elapsedSec / 60);
                    const elapsedStr = elapsedMin > 0 ? elapsedMin + 'm ' + (elapsedSec % 60) + 's' : elapsedSec + 's';
                    const brokerIdShort = msg.brokerMessageId ? msg.brokerMessageId.substring(0, 12) + '...' : 'N/A';

                    return '<tr class="hover:bg-gray-50">' +
                        '<td class="px-6 py-4 text-sm text-blue-600 font-mono max-w-xs truncate" title="' + msg.messageId + '">' + msg.messageId + '</td>' +
                        '<td class="px-6 py-4 text-sm text-gray-500 font-mono max-w-xs truncate" title="' + (msg.brokerMessageId || 'N/A') + '">' + brokerIdShort + '</td>' +
                        '<td class="px-6 py-4 text-sm text-gray-900">' + msg.queueId + '</td>' +
                        '<td class="px-6 py-4 text-sm text-gray-900">' + (msg.poolCode || 'N/A') + '</td>' +
                        '<td class="px-6 py-4 text-sm text-gray-900">' + elapsedStr + '</td>' +
                        '<td class="px-6 py-4 text-sm text-gray-500">' + addedAt.toLocaleString() + '</td>' +
                    '</tr>';
                }).join('');
            }

            filterInFlightMessages(filter) {
                if (!this.data.inFlightMessages) {
                    return;
                }

                let filtered = this.data.inFlightMessages;
                if (filter && filter.trim()) {
                    const lowerFilter = filter.toLowerCase();
                    filtered = filtered.filter(msg =>
                        msg.messageId.toLowerCase().includes(lowerFilter) ||
                        msg.queueId.toLowerCase().includes(lowerFilter)
                    );
                }

                this.renderInFlightMessages(filtered);
            }

            showHealthModal() {
                const modal = document.getElementById('healthModal');
                const modalContent = document.getElementById('modalContent');
                const health = this.data.health;

                if (!health) {
                    modalContent.innerHTML = '<p class="text-gray-600">No health information available</p>';
                } else if (health.status === 'HEALTHY') {
                    modalContent.innerHTML = '<p class="text-green-600 font-medium">✓ System is operating normally with no degradation</p>';
                } else {
                    const reasons = health.details?.degradationReason || 'No details available';
                    const reasonsList = reasons ? reasons.split('; ').map(r => '<li class="text-red-600">• ' + r + '</li>').join('') : '<li class="text-gray-600">No degradation reasons available</li>';

                    modalContent.innerHTML =
                        '<div class="space-y-3">' +
                            '<p class="font-semibold text-gray-900">Reasons for ' + health.status + ' status:</p>' +
                            '<ul class="space-y-2">' + reasonsList + '</ul>' +
                            '<p class="text-sm text-gray-600 mt-4 pt-4 border-t border-gray-200">' +
                                '<strong>Details:</strong><br>' +
                                'Queues: ' + (health.details?.totalQueues || 0) + ' total, ' + (health.details?.healthyQueues || 0) + ' healthy<br>' +
                                'Pools: ' + (health.details?.totalPools || 0) + ' total, ' + (health.details?.healthyPools || 0) + ' healthy<br>' +
                                'Active Warnings: ' + (health.details?.activeWarnings || 0) + ' (' + (health.details?.criticalWarnings || 0) + ' critical)<br>' +
                                'Circuit Breakers Open: ' + (health.details?.circuitBreakersOpen || 0) +
                            '</p>' +
                        '</div>';
                }

                modal.classList.remove('hidden');
            }

            startPeriodicRefresh() {
                setInterval(() => {
                    this.loadData();
                }, 5000); // Refresh every 5 seconds
            }
        }

        // Initialize dashboard
        const dashboard = new Dashboard();
    </script>
</body>
</html>`
