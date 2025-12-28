// Live Logs System
(function () {
    const MAX_LOG_ENTRIES_PER_SOURCE = 500;
    let logEntriesBySource = new Map(); // Per-source log storage
    let currentSource = 'koolo';
    let activeFilters = ['INFO', 'WARN', 'ERROR', 'DEBUG'];
    let searchFilter = '';
    let knownSources = new Set(['koolo']);

    // Get or create log array for a source
    function getSourceLogs(source) {
        if (!logEntriesBySource.has(source)) {
            logEntriesBySource.set(source, []);
        }
        return logEntriesBySource.get(source);
    }

    // Initialize when DOM is ready
    document.addEventListener('DOMContentLoaded', function () {
        initLogsPanel();
        loadInitialLogs();
    });

    function initLogsPanel() {
        // Tab click handlers
        document.getElementById('logs-tabs')?.addEventListener('click', function (e) {
            if (e.target.classList.contains('log-tab')) {
                setActiveSource(e.target.dataset.source);
            }
        });
    }

    function loadInitialLogs() {
        reloadLogsFromServer();
    }

    // Reload logs from the server - can be called on WebSocket reconnect
    function reloadLogsFromServer() {
        fetch(`/api/logs?source=${currentSource}&last=500`)
            .then(r => r.json())
            .then(logs => {
                if (Array.isArray(logs)) {
                    // Merge with existing logs to avoid duplicates
                    const existingLogs = getSourceLogs(currentSource);
                    const existingTimestamps = new Set(existingLogs.map(e => e.timestamp + e.message));
                    
                    logs.forEach(log => {
                        const key = log.timestamp + log.message;
                        if (!existingTimestamps.has(key)) {
                            addLogEntry(log, false);
                        }
                    });
                    renderLogs();
                }
            })
            .catch(err => console.error('Failed to load logs:', err));

        // Load available sources
        fetch('/api/logs/sources')
            .then(r => r.json())
            .then(sources => {
                if (Array.isArray(sources)) {
                    sources.forEach(s => addSourceTab(s));
                }
            })
            .catch(err => console.error('Failed to load log sources:', err));
    }

    // Export for use in dashboard.js on WebSocket reconnect
    window.reloadLogsFromServer = reloadLogsFromServer;

    // Called from WebSocket message handler
    window.handleLogMessage = function (data) {
        if (data.type === 'log' && data.data) {
            addLogEntry(data.data, true);
            addSourceTab(data.data.source);
        }
    };

    function addLogEntry(entry, render = true) {
        const source = entry.source || 'koolo';
        const sourceLogs = getSourceLogs(source);
        sourceLogs.push(entry);

        // Trim old entries for THIS source only
        if (sourceLogs.length > MAX_LOG_ENTRIES_PER_SOURCE) {
            logEntriesBySource.set(source, sourceLogs.slice(-MAX_LOG_ENTRIES_PER_SOURCE));
        }

        if (render && source === currentSource) {
            appendLogToDOM(entry);
            updateLogCount();
        }
    }

    function addSourceTab(source) {
        if (!source || knownSources.has(source)) return;
        knownSources.add(source);

        const tabsContainer = document.getElementById('logs-tabs');
        if (!tabsContainer) return;

        const tab = document.createElement('button');
        tab.className = 'log-tab';
        tab.dataset.source = source;
        tab.textContent = source;
        tabsContainer.appendChild(tab);
    }

    function setActiveSource(source) {
        currentSource = source;

        // Update tab UI
        document.querySelectorAll('.log-tab').forEach(tab => {
            tab.classList.toggle('active', tab.dataset.source === source);
        });

        renderLogs();
    }

    function renderLogs() {
        const output = document.getElementById('logs-output');
        if (!output) return;

        output.innerHTML = '';

        const sourceLogs = getSourceLogs(currentSource);
        const filtered = sourceLogs.filter(e =>
            activeFilters.includes(e.level.toUpperCase()) &&
            (searchFilter === '' || e.message.toLowerCase().includes(searchFilter.toLowerCase()))
        );

        filtered.forEach(entry => appendLogToDOM(entry, false));
        updateLogCount();

        if (document.getElementById('logs-autoscroll')?.checked) {
            output.scrollTop = output.scrollHeight;
        }
    }

    function appendLogToDOM(entry, autoScroll = true) {
        const output = document.getElementById('logs-output');
        if (!output) return;

        const levelUpper = entry.level.toUpperCase();

        // Check filters
        if (!activeFilters.includes(levelUpper)) return;
        if (searchFilter && !entry.message.toLowerCase().includes(searchFilter.toLowerCase())) return;

        const div = document.createElement('div');
        div.className = `log-entry ${levelUpper.toLowerCase()}-entry`;
        div.innerHTML = `
            <span class="log-timestamp">${escapeHtml(entry.timestamp)}</span>
            <span class="log-level ${levelUpper.toLowerCase()}">${levelUpper}</span>
            <span class="log-message">${escapeHtml(entry.message)}</span>
        `;
        output.appendChild(div);

        // Limit DOM entries
        while (output.children.length > MAX_LOG_ENTRIES_PER_SOURCE) {
            output.removeChild(output.firstChild);
        }

        if (autoScroll && document.getElementById('logs-autoscroll')?.checked) {
            output.scrollTop = output.scrollHeight;
        }
    }

    function updateLogCount() {
        const countEl = document.getElementById('log-count');
        if (countEl) {
            const sourceLogs = getSourceLogs(currentSource);
            countEl.textContent = sourceLogs.length;
        }
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Global functions for HTML onclick handlers
    window.toggleLogsPanel = function () {
        const panel = document.getElementById('logs-panel');
        const content = panel?.querySelector('.logs-content');
        if (panel && content) {
            panel.classList.toggle('collapsed');
            content.style.display = content.style.display === 'none' ? 'block' : 'none';
        }
    };

    window.clearLogs = function () {
        logEntriesBySource.set(currentSource, []);
        renderLogs();
    };

    window.updateLogFilters = function () {
        activeFilters = Array.from(document.querySelectorAll('.log-filters input:checked'))
            .map(cb => cb.value);
        renderLogs();
    };

    window.filterLogs = function () {
        const searchInput = document.getElementById('log-search');
        searchFilter = searchInput?.value || '';
        renderLogs();
    };
})();
