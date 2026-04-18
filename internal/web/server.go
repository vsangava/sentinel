package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/vsangava/distractions-free/internal/config"
	"github.com/vsangava/distractions-free/internal/testcli"
)

//go:embed static/*
var webFiles embed.FS

// ConfigHandler is a testable handler that returns the current config as JSON.
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg := config.GetConfig()
	json.NewEncoder(w).Encode(cfg)
}

// TestQueryHandler handles test queries for the web UI.
func TestQueryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Get query parameters
	timeStr := r.URL.Query().Get("time")
	domain := r.URL.Query().Get("domain")
	
	if timeStr == "" || domain == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Missing time or domain parameter",
		})
		return
	}
	
	// Get query result
	result := testcli.GetQueryResult(timeStr, domain)
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// TestPageHandler serves the test UI HTML page.
func TestPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Distractions-Free Test Query</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 900px;
            width: 100%;
            padding: 40px;
        }
        h1 {
            color: #333;
            margin-bottom: 10px;
            font-size: 28px;
        }
        .subtitle {
            color: #666;
            margin-bottom: 30px;
            font-size: 14px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            color: #333;
            font-weight: 600;
            margin-bottom: 8px;
            font-size: 14px;
        }
        input[type="text"],
        textarea {
            width: 100%;
            padding: 12px;
            border: 2px solid #e0e0e0;
            border-radius: 6px;
            font-family: monospace;
            font-size: 14px;
            transition: border-color 0.3s;
        }
        input[type="text"]:focus,
        textarea:focus {
            outline: none;
            border-color: #667eea;
            background: #f9f9ff;
        }
        textarea {
            resize: vertical;
            min-height: 120px;
        }
        .input-row {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
        }
        button {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 12px 32px;
            border-radius: 6px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 20px rgba(102, 126, 234, 0.3);
        }
        button:active {
            transform: translateY(0);
        }
        .result-section {
            margin-top: 40px;
            padding-top: 40px;
            border-top: 2px solid #e0e0e0;
        }
        .result-section.hidden {
            display: none;
        }
        .result-item {
            display: grid;
            grid-template-columns: 120px 1fr;
            gap: 20px;
            margin-bottom: 16px;
        }
        .result-label {
            font-weight: 600;
            color: #667eea;
            font-size: 14px;
        }
        .result-value {
            color: #333;
            font-family: monospace;
            font-size: 14px;
            word-break: break-all;
        }
        .status-blocked {
            color: #dc3545;
            font-weight: bold;
        }
        .status-allowed {
            color: #28a745;
            font-weight: bold;
        }
        .rules-list {
            background: #f9f9ff;
            padding: 16px;
            border-radius: 6px;
            margin-top: 8px;
        }
        .rule-item {
            margin-bottom: 12px;
            padding-bottom: 12px;
            border-bottom: 1px solid #e0e0e0;
        }
        .rule-item:last-child {
            border-bottom: none;
            margin-bottom: 0;
            padding-bottom: 0;
        }
        .schedule-slot {
            margin-top: 8px;
            padding: 8px;
            background: white;
            border-left: 3px solid #667eea;
            border-radius: 3px;
            font-size: 13px;
        }
        .schedule-active {
            color: #28a745;
            font-weight: bold;
        }
        .schedule-inactive {
            color: #999;
        }
        .warning-banner {
            background: #fff3cd;
            border: 2px solid #ffc107;
            color: #856404;
            padding: 12px;
            border-radius: 6px;
            margin-top: 16px;
            font-weight: 600;
        }
        .error-banner {
            background: #f8d7da;
            border: 2px solid #f5c6cb;
            color: #721c24;
            padding: 12px;
            border-radius: 6px;
            margin-top: 16px;
            font-weight: 600;
        }
        .loading {
            display: none;
            text-align: center;
            margin-top: 20px;
        }
        .spinner {
            border: 3px solid #f3f3f3;
            border-top: 3px solid #667eea;
            border-radius: 50%;
            width: 32px;
            height: 32px;
            animation: spin 1s linear infinite;
            display: inline-block;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        .time-format-hint {
            font-size: 12px;
            color: #999;
            margin-top: 4px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🧪 Distractions-Free Test Query</h1>
        <p class="subtitle">Test whether domains would be blocked at specific times</p>
        
        <div class="form-group">
            <label for="time">Time (Format: YYYY-MM-DD HH:MM)</label>
            <input 
                type="text" 
                id="time" 
                placeholder="Example: 2024-04-01 10:30"
                value=""
            >
            <div class="time-format-hint">Use 24-hour format (e.g., 2024-04-01 10:30 for April 1, 2024 at 10:30 AM)</div>
        </div>
        
        <div class="form-group">
            <label for="domain">Domain</label>
            <input 
                type="text" 
                id="domain" 
                placeholder="Example: youtube.com"
            >
        </div>
        
        <div class="form-group">
            <label for="config">Config (JSON)</label>
            <textarea 
                id="config" 
                placeholder="Current config will be displayed here"
                readonly
            ></textarea>
        </div>
        
        <button onclick="submitQuery()">Test Query</button>
        
        <div class="loading" id="loading">
            <div class="spinner"></div>
            <p>Running test query...</p>
        </div>
        
        <div class="result-section hidden" id="resultSection">
            <h2 style="margin-bottom: 20px; color: #333;">Query Result</h2>
            
            <div class="result-item">
                <div class="result-label">Time</div>
                <div class="result-value" id="resultTime">-</div>
            </div>
            
            <div class="result-item">
                <div class="result-label">Weekday</div>
                <div class="result-value" id="resultWeekday">-</div>
            </div>
            
            <div class="result-item">
                <div class="result-label">Domain</div>
                <div class="result-value" id="resultDomain">-</div>
            </div>
            
            <div class="result-item">
                <div class="result-label">Status</div>
                <div class="result-value" id="resultStatus">-</div>
            </div>
            
            <div class="result-item">
                <div class="result-label">DNS Response</div>
                <div class="result-value" id="resultDNS">-</div>
            </div>
            
            <div class="result-item">
                <div class="result-label">Applicable Rules</div>
                <div class="result-value">
                    <div class="rules-list" id="resultRules">No rules apply</div>
                </div>
            </div>
            
            <div id="warningSection"></div>
            <div id="errorSection"></div>
        </div>
    </div>
    
    <script>
        // Load config on page load
        window.onload = async function() {
            await loadConfig();
            setDefaultTime();
        };
        
        async function loadConfig() {
            try {
                const response = await fetch('/api/config');
                const config = await response.json();
                document.getElementById('config').value = JSON.stringify(config, null, 2);
            } catch (error) {
                console.error('Error loading config:', error);
            }
        }
        
        function setDefaultTime() {
            const now = new Date();
            const year = now.getFullYear();
            const month = String(now.getMonth() + 1).padStart(2, '0');
            const day = String(now.getDate()).padStart(2, '0');
            const hours = String(now.getHours()).padStart(2, '0');
            const minutes = String(now.getMinutes()).padStart(2, '0');
            const timeStr = year + '-' + month + '-' + day + ' ' + hours + ':' + minutes;
            document.getElementById('time').value = timeStr;
        }
        
        async function submitQuery() {
            const time = document.getElementById('time').value.trim();
            const domain = document.getElementById('domain').value.trim();
            
            if (!time || !domain) {
                alert('Please enter both time and domain');
                return;
            }
            
            document.getElementById('loading').style.display = 'block';
            document.getElementById('resultSection').classList.add('hidden');
            
            try {
                const url = '/api/test-query?time=' + encodeURIComponent(time) + 
                           '&domain=' + encodeURIComponent(domain);
                const response = await fetch(url);
                const result = await response.json();
                
                displayResult(result);
            } catch (error) {
                console.error('Error:', error);
                alert('Error running query: ' + error.message);
            } finally {
                document.getElementById('loading').style.display = 'none';
            }
        }
        
        function displayResult(result) {
            document.getElementById('resultTime').textContent = result.time;
            document.getElementById('resultWeekday').textContent = result.weekday;
            document.getElementById('resultDomain').textContent = result.domain;
            
            if (result.error) {
                const errorSection = document.getElementById('errorSection');
                errorSection.innerHTML = '<div class="error-banner">' + result.error + '</div>';
                document.getElementById('resultSection').classList.remove('hidden');
                return;
            }
            
            // Set status
            const statusEl = document.getElementById('resultStatus');
            if (result.is_blocked) {
                statusEl.innerHTML = '<span class="status-blocked">' + result.blocking_status + '</span>';
            } else {
                statusEl.innerHTML = '<span class="status-allowed">' + result.blocking_status + '</span>';
            }
            
            document.getElementById('resultDNS').textContent = result.dns_response;
            
            // Display rules
            const rulesEl = document.getElementById('resultRules');
            if (result.applicable_rules && result.applicable_rules.length > 0) {
                let rulesHTML = '';
                for (const rule of result.applicable_rules) {
                    rulesHTML += '<div class="rule-item"><strong>' + rule.domain + '</strong>';
                    for (const schedule of rule.schedules) {
                        const activeClass = schedule.is_active ? 'schedule-active' : 'schedule-inactive';
                        const activeText = schedule.is_active ? '✓ ACTIVE' : '○ Not active';
                        rulesHTML += '<div class="schedule-slot ' + activeClass + '">' +
                                    activeText + ': ' + schedule.weekday + ' ' + 
                                    schedule.start + '-' + schedule.end + '</div>';
                    }
                    rulesHTML += '</div>';
                }
                rulesEl.innerHTML = rulesHTML;
            } else {
                rulesEl.innerHTML = 'No rules apply';
            }
            
            // Display warning
            const warningEl = document.getElementById('warningSection');
            if (result.has_warning) {
                warningEl.innerHTML = '<div class="warning-banner">' + result.warning_message + '</div>';
            } else {
                warningEl.innerHTML = '';
            }
            
            document.getElementById('resultSection').classList.remove('hidden');
        }
        
        // Allow Enter key to submit
        document.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && (e.target.id === 'time' || e.target.id === 'domain')) {
                submitQuery();
            }
        });
    </script>
</body>
</html>`
	w.Write([]byte(html))
}

// StaticFileHandler returns a handler for serving embedded static files.
func StaticFileHandler() (http.Handler, error) {
	fsys, err := fs.Sub(webFiles, "static")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(fsys)), nil
}

func StartWebServer() {
	staticHandler, err := StaticFileHandler()
	if err != nil {
		log.Fatalf("Failed to load embedded web files: %v", err)
	}

	http.Handle("/", staticHandler)
	http.HandleFunc("/api/config", ConfigHandler)
	http.HandleFunc("/api/test-query", TestQueryHandler)

	log.Println("Web server starting on http://localhost:8040")
	if err := http.ListenAndServe("127.0.0.1:8040", nil); err != nil {
		log.Fatalf("Web server failed: %v", err)
	}
}

// StartTestWebServer starts a web server dedicated to test queries.
func StartTestWebServer() {
	http.HandleFunc("/", TestPageHandler)
	http.HandleFunc("/api/config", ConfigHandler)
	http.HandleFunc("/api/test-query", TestQueryHandler)

	log.Println("Test web server starting on http://localhost:8040")
	if err := http.ListenAndServe("127.0.0.1:8040", nil); err != nil {
		log.Fatalf("Test web server failed: %v", err)
	}
}
