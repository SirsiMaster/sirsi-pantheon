package mirror

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"sync"
)

// Server runs a local web UI for the Mirror dedup scanner.
type Server struct {
	port     int
	listener net.Listener
	mu       sync.Mutex
	result   *MirrorResult
	scanning bool
}

// NewServer creates a Mirror web UI server on a random available port.
func NewServer() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return &Server{port: port, listener: ln}, nil
}

// URL returns the local URL for the web UI.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

// OpenBrowser opens the default browser to the web UI.
func (s *Server) OpenBrowser() error {
	url := s.URL()
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// Serve starts the HTTP server (blocking).
func (s *Server) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleUI)
	mux.HandleFunc("/api/scan", s.handleScan)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/result", s.handleResult)
	return http.Serve(s.listener, mux)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Paths   []string `json:"paths"`
		MinSize int64    `json:"min_size"`
		Filter  string   `json:"filter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "already_scanning"})
		return
	}
	s.scanning = true
	s.mu.Unlock()

	go func() {
		opts := ScanOptions{
			Paths:   req.Paths,
			MinSize: req.MinSize,
		}
		if req.Filter != "" {
			opts.MediaFilter = MediaType(req.Filter)
		}

		result, err := Scan(opts)
		s.mu.Lock()
		if err == nil {
			s.result = result
		}
		s.scanning = false
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"scanning":   s.scanning,
		"has_result": s.result != nil,
	})
}

func (s *Server) handleResult(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if s.result == nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "no_result"})
		return
	}
	_ = json.NewEncoder(w).Encode(s.result)
}

func (s *Server) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, mirrorHTML())
}

func mirrorHTML() string {
	// Pre-sort media extensions for the filter chips
	photoExts := []string{}
	musicExts := []string{}
	for ext, mt := range mediaExtensions {
		switch mt {
		case MediaPhoto:
			photoExts = append(photoExts, ext)
		case MediaMusic:
			musicExts = append(musicExts, ext)
		}
	}
	sort.Strings(photoExts)
	sort.Strings(musicExts)

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>𓂀 Anubis Mirror — Duplicate File Scanner</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #0A0A12;
  --surface: #12121E;
  --surface2: #1A1A2E;
  --border: rgba(200,169,81,0.1);
  --border-hover: rgba(200,169,81,0.3);
  --gold: #C8A951;
  --gold-dim: rgba(200,169,81,0.4);
  --text: #E0E0E0;
  --text-dim: #666;
  --green: #2ECC71;
  --red: #E74C3C;
  --blue: #4A90D9;
  --orange: #F39C12;
  --radius: 16px;
  --radius-sm: 10px;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: 'Inter', -apple-system, system-ui, sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
  overflow-x: hidden;
}

/* --- Layout --- */
.container { max-width: 960px; margin: 0 auto; padding: 0 24px; }

header {
  padding: 32px 0 16px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 32px;
}
header h1 {
  font-size: 22px; font-weight: 300; letter-spacing: 3px;
  text-transform: uppercase; color: var(--gold);
}
header p { font-size: 12px; color: var(--text-dim); margin-top: 6px; letter-spacing: 0.5px; }

/* --- Drop Zone --- */
#drop-zone {
  border: 2px dashed var(--border);
  border-radius: var(--radius);
  padding: 80px 40px;
  text-align: center;
  cursor: pointer;
  transition: all 0.4s cubic-bezier(0.4, 0, 0.2, 1);
  background: var(--surface);
  position: relative;
  overflow: hidden;
}
#drop-zone::before {
  content: '';
  position: absolute; inset: 0;
  background: radial-gradient(circle at center, rgba(200,169,81,0.03) 0%, transparent 70%);
  transition: opacity 0.4s;
}
#drop-zone:hover, #drop-zone.drag-over {
  border-color: var(--gold);
  background: var(--surface2);
  transform: scale(1.01);
  box-shadow: 0 0 60px rgba(200,169,81,0.08);
}
#drop-zone:hover::before, #drop-zone.drag-over::before { opacity: 2; }
#drop-zone .icon { font-size: 56px; margin-bottom: 16px; display: block; }
#drop-zone .title {
  font-size: 20px; font-weight: 500; color: var(--text); margin-bottom: 8px;
}
#drop-zone .subtitle {
  font-size: 13px; color: var(--text-dim); line-height: 1.6;
}
#drop-zone .browse-btn {
  display: inline-block; margin-top: 20px;
  padding: 10px 28px; border-radius: 8px;
  background: rgba(200,169,81,0.1); border: 1px solid var(--border-hover);
  color: var(--gold); font-size: 13px; font-weight: 500;
  cursor: pointer; transition: all 0.3s;
  letter-spacing: 0.5px;
}
#drop-zone .browse-btn:hover {
  background: rgba(200,169,81,0.2); transform: translateY(-1px);
  box-shadow: 0 4px 16px rgba(200,169,81,0.1);
}

/* --- Folder List --- */
#folder-list {
  margin-top: 24px; display: flex; flex-direction: column; gap: 8px;
}
.folder-chip {
  display: flex; align-items: center; gap: 10px;
  background: var(--surface2); border: 1px solid var(--border);
  border-radius: var(--radius-sm); padding: 12px 16px;
  font-size: 13px; animation: slideIn 0.3s ease;
}
.folder-chip .path { flex: 1; color: var(--text-dim); font-family: 'SF Mono', monospace; font-size: 12px; }
.folder-chip .remove {
  color: var(--text-dim); cursor: pointer; font-size: 16px;
  transition: color 0.2s; width: 24px; height: 24px;
  display: flex; align-items: center; justify-content: center;
  border-radius: 6px;
}
.folder-chip .remove:hover { color: var(--red); background: rgba(231,76,60,0.1); }

@keyframes slideIn {
  from { opacity: 0; transform: translateY(-8px); }
  to { opacity: 1; transform: translateY(0); }
}

/* --- Scan Button --- */
#scan-section { margin-top: 24px; display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
#scan-btn {
  padding: 14px 40px; border-radius: var(--radius-sm);
  background: linear-gradient(135deg, #C8A951, #A88B34);
  border: none; color: #0A0A12; font-size: 14px; font-weight: 600;
  cursor: pointer; transition: all 0.3s; letter-spacing: 0.5px;
  box-shadow: 0 4px 20px rgba(200,169,81,0.2);
}
#scan-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 30px rgba(200,169,81,0.3);
}
#scan-btn:disabled { opacity: 0.4; cursor: not-allowed; transform: none; box-shadow: none; }

.filter-chip {
  padding: 8px 16px; border-radius: 20px;
  background: var(--surface2); border: 1px solid var(--border);
  color: var(--text-dim); font-size: 12px; cursor: pointer;
  transition: all 0.2s;
}
.filter-chip:hover, .filter-chip.active {
  border-color: var(--gold); color: var(--gold);
  background: rgba(200,169,81,0.08);
}

/* --- Scanning State --- */
#scanning {
  display: none; text-align: center; padding: 80px 20px;
}
#scanning .spinner {
  width: 56px; height: 56px; border-radius: 50%;
  border: 3px solid var(--border);
  border-top-color: var(--gold);
  animation: spin 1s linear infinite;
  margin: 0 auto 24px;
}
@keyframes spin { to { transform: rotate(360deg); } }
#scanning .label { font-size: 16px; color: var(--text-dim); }

/* --- Results --- */
#results { display: none; }

.stats-bar {
  display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px;
  margin-bottom: 32px;
}
.stat-card {
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius-sm); padding: 20px;
  text-align: center;
}
.stat-card .value { font-size: 28px; font-weight: 600; color: var(--gold); }
.stat-card .label { font-size: 11px; color: var(--text-dim); margin-top: 4px; text-transform: uppercase; letter-spacing: 1px; }

.dup-group {
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius); margin-bottom: 12px;
  overflow: hidden; transition: border-color 0.2s;
}
.dup-group:hover { border-color: var(--border-hover); }

.dup-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 16px 20px; border-bottom: 1px solid var(--border);
  cursor: pointer;
}
.dup-header .group-title { font-size: 13px; font-weight: 500; }
.dup-header .waste-badge {
  font-size: 11px; padding: 4px 12px; border-radius: 20px;
  background: rgba(231,76,60,0.1); color: var(--red);
  font-weight: 500;
}

.dup-files { padding: 8px 0; }
.dup-file {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 20px; transition: background 0.15s;
}
.dup-file:hover { background: var(--surface2); }

.file-status {
  width: 32px; height: 32px; border-radius: 8px;
  display: flex; align-items: center; justify-content: center;
  font-size: 14px; flex-shrink: 0;
}
.file-status.keep { background: rgba(46,204,113,0.1); color: var(--green); }
.file-status.remove { background: rgba(231,76,60,0.1); color: var(--red); }

.file-info { flex: 1; min-width: 0; }
.file-name {
  font-size: 13px; font-weight: 500; white-space: nowrap;
  overflow: hidden; text-overflow: ellipsis;
}
.file-path {
  font-size: 11px; color: var(--text-dim); margin-top: 2px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  font-family: 'SF Mono', monospace;
}
.file-meta {
  display: flex; gap: 12px; flex-shrink: 0;
}
.file-meta span { font-size: 11px; color: var(--text-dim); }

.file-action {
  padding: 6px 14px; border-radius: 6px;
  font-size: 11px; font-weight: 500; cursor: pointer;
  border: 1px solid; transition: all 0.2s;
  flex-shrink: 0;
}
.file-action.keeping {
  border-color: rgba(46,204,113,0.3); color: var(--green);
  background: rgba(46,204,113,0.05);
}
.file-action.removing {
  border-color: rgba(231,76,60,0.3); color: var(--red);
  background: rgba(231,76,60,0.05);
}
.file-action:hover { transform: scale(1.05); }

/* --- No Duplicates --- */
.clean-state {
  text-align: center; padding: 80px 20px;
}
.clean-state .icon { font-size: 64px; margin-bottom: 16px; }
.clean-state .title { font-size: 22px; font-weight: 500; color: var(--green); }
.clean-state .subtitle { font-size: 14px; color: var(--text-dim); margin-top: 8px; }

/* --- Footer --- */
footer {
  text-align: center; padding: 40px 0;
  font-size: 10px; color: #333; letter-spacing: 1px;
}

/* --- Responsive --- */
@media (max-width: 640px) {
  .stats-bar { grid-template-columns: repeat(2, 1fr); }
  #drop-zone { padding: 48px 24px; }
}
</style>
</head>
<body>
<div class="container">
  <header>
    <h1>𓂀 Mirror</h1>
    <p>Find duplicate files • Smart keep recommendations • Zero data leaves your device</p>
  </header>

  <!-- Step 1: Drop Zone -->
  <div id="step-select">
    <div id="drop-zone" onclick="selectFolders()">
      <span class="icon">🪞</span>
      <div class="title">Drop folders here</div>
      <div class="subtitle">or click to browse — Mirror will scan for duplicates</div>
      <div class="browse-btn">Choose Folders</div>
    </div>
    <div id="folder-list"></div>
    <div id="scan-section" style="display:none">
      <button id="scan-btn" onclick="startScan()">Scan for Duplicates</button>
      <button class="filter-chip" data-filter="" onclick="setFilter(this)">All Files</button>
      <button class="filter-chip" data-filter="photo" onclick="setFilter(this)">📷 Photos</button>
      <button class="filter-chip" data-filter="music" onclick="setFilter(this)">🎵 Music</button>
    </div>
  </div>

  <!-- Step 2: Scanning -->
  <div id="scanning">
    <div class="spinner"></div>
    <div class="label">Scanning for duplicates...</div>
  </div>

  <!-- Step 3: Results -->
  <div id="results"></div>

  <footer>𓂀 Sirsi Anubis • Mirror — all analysis stays on-device</footer>
</div>

<script>
const folders = [];
let activeFilter = '';

// Drag & drop
const dz = document.getElementById('drop-zone');
['dragenter','dragover'].forEach(e => dz.addEventListener(e, ev => {
  ev.preventDefault(); dz.classList.add('drag-over');
}));
['dragleave','drop'].forEach(e => dz.addEventListener(e, ev => {
  ev.preventDefault(); dz.classList.remove('drag-over');
}));
dz.addEventListener('drop', ev => {
  const items = ev.dataTransfer.items;
  if (items) {
    for (let i = 0; i < items.length; i++) {
      const entry = items[i].webkitGetAsEntry && items[i].webkitGetAsEntry();
      if (entry && entry.isDirectory) {
        addFolder(entry.fullPath || entry.name);
      }
    }
  }
});

function selectFolders() {
  const input = document.createElement('input');
  input.type = 'file';
  input.webkitdirectory = true;
  input.multiple = true;
  input.addEventListener('change', () => {
    if (input.files.length > 0) {
      // Extract directory path from first file
      const firstFile = input.files[0];
      const dir = firstFile.webkitRelativePath.split('/')[0];
      addFolder(dir);
    }
  });
  input.click();
}

function addFolder(path) {
  if (folders.includes(path)) return;
  folders.push(path);
  renderFolders();
}

function removeFolder(idx) {
  folders.splice(idx, 1);
  renderFolders();
}

function renderFolders() {
  const list = document.getElementById('folder-list');
  const scanSec = document.getElementById('scan-section');
  if (folders.length === 0) {
    list.innerHTML = '';
    scanSec.style.display = 'none';
    return;
  }
  scanSec.style.display = 'flex';
  list.innerHTML = folders.map((f, i) =>
    '<div class="folder-chip">' +
    '<span>📂</span>' +
    '<span class="path">' + escapeHtml(f) + '</span>' +
    '<span class="remove" onclick="removeFolder(' + i + ')">✕</span>' +
    '</div>'
  ).join('');
  document.querySelectorAll('.filter-chip')[0].click();
}

function setFilter(btn) {
  document.querySelectorAll('.filter-chip').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  activeFilter = btn.dataset.filter;
}

async function startScan() {
  if (folders.length === 0) return;
  document.getElementById('step-select').style.display = 'none';
  document.getElementById('scanning').style.display = 'block';
  document.getElementById('results').style.display = 'none';

  try {
    await fetch('/api/scan', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ paths: folders, filter: activeFilter })
    });

    // Poll for completion
    const poll = setInterval(async () => {
      const res = await fetch('/api/status');
      const data = await res.json();
      if (!data.scanning && data.has_result) {
        clearInterval(poll);
        const resultRes = await fetch('/api/result');
        const result = await resultRes.json();
        showResults(result);
      }
    }, 300);
  } catch (e) {
    document.getElementById('scanning').style.display = 'none';
    document.getElementById('step-select').style.display = 'block';
    alert('Scan failed: ' + e.message);
  }
}

function showResults(r) {
  document.getElementById('scanning').style.display = 'none';
  document.getElementById('results').style.display = 'block';

  if (!r.groups || r.groups.length === 0) {
    document.getElementById('results').innerHTML =
      '<div class="clean-state">' +
      '<div class="icon">✅</div>' +
      '<div class="title">No Duplicates Found</div>' +
      '<div class="subtitle">Your files are clean — nothing wasted!</div>' +
      '</div>';
    return;
  }

  let html = '<div class="stats-bar">';
  html += statCard(r.total_scanned, 'Files Scanned');
  html += statCard(r.total_duplicates, 'Duplicates');
  html += statCard(formatBytes(r.total_waste_bytes), 'Wasted Space');
  html += statCard(r.scan_duration ? (r.scan_duration / 1e6).toFixed(0) + 'ms' : '—', 'Scan Time');
  html += '</div>';

  r.groups.forEach((g, gi) => {
    html += '<div class="dup-group">';
    html += '<div class="dup-header" onclick="toggleGroup(' + gi + ')">';
    html += '<span class="group-title">' + mediaEmoji(g.files[0]) + ' ' + escapeHtml(basename(g.files[g.recommended].path)) + ' <span style="color:var(--text-dim);font-weight:300">× ' + g.files.length + '</span></span>';
    html += '<span class="waste-badge">−' + formatBytes(g.waste_bytes) + '</span>';
    html += '</div>';
    html += '<div class="dup-files" id="group-' + gi + '">';
    g.files.forEach((f, fi) => {
      const isKeep = fi === g.recommended;
      html += '<div class="dup-file">';
      html += '<div class="file-status ' + (isKeep ? 'keep' : 'remove') + '">' + (isKeep ? '✓' : '✗') + '</div>';
      html += '<div class="file-info">';
      html += '<div class="file-name">' + escapeHtml(basename(f.path)) + '</div>';
      html += '<div class="file-path">' + escapeHtml(shortenPath(f.path)) + '</div>';
      html += '</div>';
      html += '<div class="file-meta">';
      html += '<span>' + formatBytes(f.size) + '</span>';
      if (f.mod_time) html += '<span>' + f.mod_time.substring(0,10) + '</span>';
      html += '</div>';
      html += '<div class="file-action ' + (isKeep ? 'keeping' : 'removing') + '">' + (isKeep ? 'Keep' : 'Remove') + '</div>';
      html += '</div>';
    });
    html += '</div></div>';
  });

  html += '<div style="text-align:center;margin:32px 0">';
  html += '<button onclick="scanAgain()" style="padding:10px 24px;border-radius:8px;background:var(--surface2);border:1px solid var(--border);color:var(--text-dim);cursor:pointer;font-size:13px;transition:all 0.2s" onmouseover="this.style.borderColor=\'var(--gold)\';this.style.color=\'var(--gold)\'" onmouseout="this.style.borderColor=\'var(--border)\';this.style.color=\'var(--text-dim)\'">← Scan Again</button>';
  html += '</div>';

  document.getElementById('results').innerHTML = html;
}

function scanAgain() {
  document.getElementById('results').style.display = 'none';
  document.getElementById('results').innerHTML = '';
  document.getElementById('step-select').style.display = 'block';
}

function toggleGroup(idx) {
  const el = document.getElementById('group-' + idx);
  el.style.display = el.style.display === 'none' ? 'block' : 'none';
}

function statCard(value, label) {
  return '<div class="stat-card"><div class="value">' + value + '</div><div class="label">' + label + '</div></div>';
}

function formatBytes(b) {
  if (!b || b === 0) return '0 B';
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
  return (b / 1073741824).toFixed(1) + ' GB';
}

function basename(path) {
  return path.split('/').pop() || path;
}

function shortenPath(path) {
  const home = '/Users/';
  const idx = path.indexOf(home);
  if (idx >= 0) {
    const after = path.substring(idx + home.length);
    const slash = after.indexOf('/');
    if (slash >= 0) return '~' + after.substring(slash);
  }
  return path;
}

function mediaEmoji(f) {
  const mt = f.media_type || 'other';
  return {photo:'📷', music:'🎵', video:'🎬', document:'📄'}[mt] || '📁';
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.appendChild(document.createTextNode(s));
  return d.innerHTML;
}
</script>
</body>
</html>`
}
