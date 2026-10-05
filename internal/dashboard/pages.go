package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
)

// ── Shared Layout ───────────────────────────────────────────────────────

// pageShell wraps page-specific content in the shared dashboard layout.
// activePage is the nav item to highlight (e.g., "/", "/scan", "/guard").
//
// port is the port the server is ACTUALLY listening on, not the package
// default. The footer renders it as this node's address, so it has to be the
// live value: rendering the DashboardPort constant made `--port 8080` serve a
// working UI that told the operator the node was on 9119. Callers pass
// s.cfg.Port, which NewServer has already normalized to DashboardPort when
// unset, so this can never render :0.
func pageShell(title, activePage, bodyContent string, port int) string {
	primaryNavItems := []struct {
		Key   string
		Label string
	}{
		{"home", "Home"},
		{"fleet", "Fleet"},
		{"engine", "Engine"},
		{"sne", "SNE"},
		{"ra", "Ra"},
	}
	toolNavItems := []struct {
		Key   string
		Label string
	}{
		{"scan", "Scan"},
		{"ghosts", "Ghosts"},
		{"guard", "Guard"},
		{"notifications", "Notifications"},
		{"horus", "Code Graph"},
		{"vault", "Vault"},
		{"recovery", "Recovery"},
	}

	var navHTML strings.Builder
	writeNavLink := func(n struct{ Key, Label string }) {
		cls := "nav-item"
		current := ""
		href := "/?view=" + n.Key
		if n.Key == "home" {
			href = "/"
		}
		if n.Key == activePage {
			cls += " active"
			current = ` aria-current="page"`
		}
		navHTML.WriteString(fmt.Sprintf(
			`<a href="%s" class="%s" data-view="%s"%s><span class="nav-label">%s</span></a>`,
			href, cls, n.Key, current, n.Label,
		))
	}
	for _, n := range primaryNavItems {
		writeNavLink(n)
	}
	toolsOpen := ""
	for _, n := range toolNavItems {
		if n.Key == activePage {
			toolsOpen = " open"
			break
		}
	}
	navHTML.WriteString(`<details class="nav-tools"` + toolsOpen + `><summary class="nav-tools-toggle">Tools</summary><div class="nav-tools-panel">`)
	for _, n := range toolNavItems {
		writeNavLink(n)
	}
	navHTML.WriteString(`</div></details>`)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s — Sirsi Pantheon</title>
<style>
:root{%s}
*{margin:0;padding:0;box-sizing:border-box}
body{background:%s;color:%s;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;
display:flex;min-height:100vh;overflow:hidden}
.skip-link{position:fixed;left:12px;top:-64px;z-index:30;padding:10px 14px;border-radius:6px;background:var(--emerald);color:var(--bg);font:600 13px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;text-decoration:none}
.skip-link:focus{top:12px;outline:3px solid var(--ink);outline-offset:2px}
::-webkit-scrollbar{width:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:color-mix(in srgb, var(--gold) 20%%, transparent);border-radius:3px}

/* Sidebar */
.sidebar{width:224px;min-height:100vh;background:var(--panel);border-right:1px solid %s;
display:flex;flex-direction:column;position:fixed;left:0;top:0;bottom:0;z-index:10}
.sidebar-brand{padding:24px 20px 20px;border-bottom:1px solid %s}
.sidebar-brand h1{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:15px;font-weight:700;
color:%s;letter-spacing:.16em;text-transform:uppercase}
.sidebar-build{max-width:176px;margin-top:6px;color:var(--dim);font-size:9px;line-height:1.4;letter-spacing:.02em;
white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.sidebar-build[data-state="unavailable"]{max-width:none;padding:7px 8px;border:1px solid color-mix(in srgb,var(--warn) 42%%,var(--line));border-radius:5px;
background:color-mix(in srgb,var(--warn) 8%%,transparent);color:var(--warn);font-size:11px;font-weight:600;white-space:normal;overflow:visible;text-overflow:clip;overflow-wrap:anywhere}
.sidebar-nav{flex:1;padding:14px 10px}
.nav-item{display:flex;align-items:center;padding:11px 12px;color:%s;text-decoration:none;
font-size:13px;font-weight:600;letter-spacing:.01em;transition:background .15s,color .15s,border-color .15s;border-left:2px solid transparent;border-radius:6px;cursor:pointer;
font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
.nav-item:hover{background:color-mix(in srgb, var(--gold) 7%%, transparent);color:%s}
.nav-item.active{background:color-mix(in srgb, var(--emerald) 10%%, transparent);color:%s;border-left-color:%s}
.nav-tools{margin-top:8px;border-top:1px solid var(--line);padding-top:8px}
.nav-tools-toggle{display:flex;align-items:center;min-height:40px;padding:10px 12px;color:%s;font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;cursor:pointer;list-style:none}
.nav-tools-toggle::-webkit-details-marker{display:none}
.nav-tools-toggle::after{content:'+';margin-left:auto;color:var(--gold);font-size:14px}
.nav-tools[open]>.nav-tools-toggle::after{content:'−'}
.nav-tools-toggle:hover{color:%s}
.nav-tools-toggle:focus-visible{outline:2px solid var(--gold);outline-offset:2px;border-radius:4px}
.nav-tools-panel{display:flex;flex-direction:column}
.nav-glyph{width:20px;font-size:14px;margin-right:8px;text-align:center}
.sidebar-footer{padding:14px 20px;border-top:1px solid %s;font-size:9px;color:var(--dim);letter-spacing:.08em;
font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}

/* Main — content is capped at 1400px and centered in the space right of the
   fixed sidebar so ultra-wide viewports don't strand everything top-left. */
.main{margin-left:224px;flex:1;display:flex;flex-direction:column;align-items:center;height:100vh;overflow:hidden}
.main-inner{width:100%%;max-width:1400px;display:flex;flex-direction:column;height:100vh;overflow:hidden;
border-left:1px solid color-mix(in srgb, var(--gold) 6%%, transparent);border-right:1px solid color-mix(in srgb, var(--gold) 6%%, transparent)}

/* Stats bar */
.stats-bar{display:flex;gap:10px;padding:12px 16px;background:var(--bg);border-bottom:1px solid %s;flex-shrink:0}
.stat{flex:1;padding:13px 15px;background:var(--panel);border:1px solid var(--line);border-radius:8px}
.stat-label{font-size:10px;color:%s;letter-spacing:.12em;text-transform:uppercase;
font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;margin-bottom:4px}
.stat-value{font-size:18px;color:%s;font-weight:650;letter-spacing:-.02em}
.stat-sub{font-size:10px;color:var(--dim);margin-top:2px}
/* Only tiles that actually go somewhere get a pointer and a chevron. A readout
   that looks clickable and isn't is worse than one that plainly isn't. */
.stat-go{cursor:pointer}
.stat-go:hover{background:color-mix(in srgb, var(--gold) 7%%, transparent)}
.stat-go:focus-visible{outline:1px solid var(--gold);outline-offset:-1px}
.stat-go .stat-label::after{content:' \203A';color:var(--gold);opacity:.6}

/* Clickable command rows (home screen) */
.t-cmd{display:flex;gap:14px;cursor:pointer;padding:2px 8px;margin:0 -8px;border-radius:3px}
.t-cmd:hover,.t-cmd:focus-visible{background:color-mix(in srgb, var(--gold) 8%%, transparent);outline:none}
.t-cmd-name{color:var(--ink2);min-width:96px;flex-shrink:0}
.t-cmd-desc{color:var(--dim)}
.t-cmd:hover .t-cmd-name,.t-cmd:focus-visible .t-cmd-name{color:var(--gold)}
.t-cmd:hover .t-cmd-desc,.t-cmd:focus-visible .t-cmd-desc{color:var(--ink2)}

/* Terminal */
.terminal-wrap{flex:1;display:flex;flex-direction:column;overflow:hidden}
.term-input-bar{display:flex;align-items:center;gap:8px;padding:10px 16px;border-bottom:1px solid %s;background:var(--bg);flex-shrink:0}
.term-prompt{color:%s;padding:0 2px;font-size:14px;font-weight:700;flex-shrink:0}
.term-input{flex:1;background:var(--panel);border:1px solid var(--line);border-radius:6px;color:%s;font-size:14px;padding:10px 12px;
font-family:inherit;outline:none}
.term-input:focus{border-color:var(--emerald);box-shadow:0 0 0 3px color-mix(in srgb, var(--emerald) 16%%, transparent)}
.term-input::placeholder{color:var(--dim)}
.term-submit{min-width:64px;min-height:44px;padding:9px 13px;border:1px solid var(--emerald);border-radius:6px;background:var(--emerald);color:var(--bg);font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer}
.term-submit:hover:not(:disabled){filter:brightness(1.08)}
.term-submit:focus-visible{outline:2px solid var(--ink);outline-offset:2px}
.term-submit:disabled{border-color:var(--line);background:var(--panel);color:var(--dim);cursor:not-allowed}
.term-cancel{padding:8px 10px;border:1px solid var(--line);border-radius:6px;background:transparent;color:var(--ink2);font:600 11px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer}
.term-cancel:hover,.term-cancel:focus-visible{border-color:var(--danger);color:var(--danger);outline:2px solid var(--danger);outline-offset:2px}
.term-cancel:disabled,.term-cancel[hidden]{display:none}
.term-view-label{color:var(--dim);font-size:10px;padding-right:16px;letter-spacing:1px;text-transform:uppercase;
font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;flex-shrink:0}
.terminal{flex:1;overflow-y:auto;padding:24px 28px;background:var(--bg);line-height:1.7;font:13px/1.7 'SF Mono',Menlo,Consolas,'Courier New',monospace}
.t-line{margin:0;white-space:pre-wrap;overflow-wrap:anywhere;word-break:normal}
.t-dim{color:var(--dim)}
.t-out{color:var(--ink2)}
.t-ok{color:var(--ok)}
.t-err{color:var(--danger)}
.t-gold{color:var(--gold)}
.t-head{color:var(--gold);font-weight:600;font-size:13px;margin-top:8px}
.t-row{display:flex;gap:16px;padding:2px 0}
.t-row:hover{background:color-mix(in srgb, var(--gold) 3%%, transparent)}
.t-col{color:var(--ink2)}.t-col-r{color:var(--gold);text-align:right;min-width:80px}
	.t-action{color:var(--dim);cursor:pointer;transition:color .15s;text-decoration:underline;text-decoration-color:var(--line)}
	.t-action:hover{color:var(--gold);text-decoration-color:var(--gold)}
	.t-action:focus-visible,.nav-item:focus-visible{color:var(--gold);outline:2px solid var(--gold);outline-offset:3px;text-decoration-color:var(--gold)}
	.worker-action-form{max-width:920px;margin:16px 0;padding:14px;border:1px solid var(--line);border-radius:6px;background:rgba(255,255,255,.025)}
	.worker-action-title{margin:0 0 5px;color:var(--ink2);font:600 14px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.worker-action-note{margin:0 0 12px;color:var(--dim);font:12px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.worker-action-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));gap:10px}
	.worker-action-field{display:flex;min-width:0;flex-direction:column;gap:5px;color:var(--ink2);font:12px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.worker-action-field.wide{grid-column:1/-1}
	.worker-action-form input,.worker-action-form select,.worker-action-form textarea{width:100%%;min-width:0;box-sizing:border-box;padding:8px 9px;border:1px solid var(--line);border-radius:4px;background:var(--bg);color:var(--ink2);font:13px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.worker-action-form textarea{min-height:82px;resize:vertical}
	.worker-action-form input:focus-visible,.worker-action-form select:focus-visible,.worker-action-form textarea:focus-visible{outline:2px solid var(--gold);outline-offset:2px}
	.worker-action-form input::placeholder,.worker-action-form textarea::placeholder{color:var(--dim);opacity:1}
	.worker-action-footer{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-top:12px}
	.worker-action-submit,.worker-action-refresh{padding:7px 11px;border:1px solid var(--gold);border-radius:4px;background:transparent;color:var(--gold);font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer}
	.worker-action-submit:hover,.worker-action-submit:focus-visible,.worker-action-refresh:hover,.worker-action-refresh:focus-visible{background:color-mix(in srgb,var(--gold) 9%%,transparent);outline:2px solid var(--gold);outline-offset:2px}
	.worker-action-submit:disabled{opacity:.55;cursor:wait}
	.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
	.t-empty-state{max-width:680px;margin:24px 0;padding:20px;border:1px solid color-mix(in srgb, var(--warn) 30%%, var(--line));border-radius:12px;background:color-mix(in srgb, var(--warn) 7%%, transparent)}
	.t-empty-title{color:var(--ink2);font:600 14px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;letter-spacing:.02em}
	.t-empty-copy{max-width:62ch;margin-top:7px;color:var(--dim);font:13px/1.55 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.t-empty-detail{margin-top:8px;color:var(--dim);font-size:11px;overflow-wrap:anywhere}
	.t-empty-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:14px}
	.t-empty-action{padding:6px 10px;border:1px solid var(--line);border-radius:4px;background:transparent;color:var(--gold);font:600 11px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer}
	.t-empty-action:hover,.t-empty-action:focus-visible{border-color:var(--gold);background:color-mix(in srgb, var(--gold) 8%%, transparent);outline:2px solid var(--gold);outline-offset:2px}
	.guard-summary{display:grid;grid-template-columns:minmax(180px,1.6fr) repeat(3,minmax(86px,1fr));gap:8px;margin:8px 0 16px}
	.guard-score-card,.guard-kpi{padding:12px;border:1px solid var(--line);border-radius:5px;background:rgba(255,255,255,.025)}
	.guard-score-card{border-top:3px solid var(--gold)}
	.guard-score-value{color:var(--ink2);font-size:20px;line-height:1.2}
	.guard-score-label,.guard-kpi-label{margin-top:4px;color:var(--dim);font:10px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;letter-spacing:.08em;text-transform:uppercase}
	.guard-kpi-value{color:var(--ink2);font-size:18px;line-height:1.2}
	.guard-kpi.critical{border-top:3px solid var(--danger)}
	.guard-kpi.warning{border-top:3px solid var(--warn)}
	.guard-kpi.healthy{border-top:3px solid var(--ok)}
	.guard-section-label{margin:8px 0;color:var(--gold);font:600 11px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;letter-spacing:.08em;text-transform:uppercase}
	.guard-list{display:flex;flex-direction:column;gap:6px}
	.guard-finding{display:grid;grid-template-columns:22px minmax(140px,220px) minmax(0,1fr);gap:10px;align-items:start;padding:9px 10px;border:1px solid var(--line);border-radius:4px;background:rgba(255,255,255,.018)}
	.guard-finding.severity-3{border-top:2px solid var(--danger);background:color-mix(in srgb, var(--danger) 7%%, transparent)}
	.guard-finding.severity-2{border-top:2px solid var(--warn);background:color-mix(in srgb, var(--warn) 6%%, transparent)}
	.guard-finding.severity-1{border-top:2px solid var(--gold)}
	.guard-finding.severity-0{border-top:2px solid var(--ok)}
	.guard-finding-icon{font-size:13px;line-height:1.4}
	.guard-finding-name{color:var(--ink2);font:600 12px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.guard-finding-message{min-width:0;color:var(--dim);font:12px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;overflow-wrap:anywhere}
	.guard-all{margin-top:12px;border-top:1px solid var(--line);padding-top:10px}
	.guard-all summary{color:var(--dim);cursor:pointer;font:600 11px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;letter-spacing:.04em}
	.guard-all summary:focus-visible{outline:2px solid var(--gold);outline-offset:3px}
	.guard-all .guard-list{margin-top:10px}
	.engine-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:10px;margin:10px 0 14px}
	.engine-card{display:flex;flex-direction:column;gap:6px;padding:12px;border:1px solid var(--line);border-radius:4px;background:rgba(255,255,255,.025);min-height:122px}
	.engine-card.selected{border-color:var(--gold);background:color-mix(in srgb, var(--gold) 8%%, transparent)}
	.engine-name{color:var(--ink2);font-size:14px;font-weight:600;letter-spacing:.08em}
	.engine-status{color:var(--dim);font-size:11px;text-transform:uppercase;letter-spacing:.08em}
	.engine-caps{color:var(--dim);font-size:11px;line-height:1.5;flex:1}
	.engine-select{align-self:flex-start;padding:4px 9px;border:1px solid var(--line);border-radius:3px;background:transparent;color:var(--gold);font:inherit;font-size:11px;cursor:pointer}
	.engine-select:hover{border-color:var(--gold);background:color-mix(in srgb, var(--gold) 8%%, transparent)}
	.engine-select:focus-visible{outline:2px solid var(--gold);outline-offset:2px}
	.engine-select:disabled{border-color:var(--gold);color:var(--gold);cursor:default;opacity:.9}
	.engine-followup{margin-top:8px}
	.engine-followup-button{min-height:40px;padding:8px 12px;border:1px solid var(--gold);border-radius:6px;background:transparent;color:var(--gold);font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer}
	.engine-followup-button:hover,.engine-followup-button:focus-visible{background:color-mix(in srgb,var(--gold) 9%%,transparent);outline:2px solid var(--gold);outline-offset:2px}
	/* The home surface is a request-and-evidence workspace. Keep the shared
	   navigation and brand, but stop presenting the first task as terminal output. */
	body.home-view .stats-bar{gap:0;padding:10px 28px;background:#f3ead6;border-bottom-color:#d7ded8}
	body.home-view .stat{padding:7px 14px;background:transparent;border:0;border-right:1px solid #d7ded8;border-radius:0}
	body.home-view .stat:last-child{border-right:0}
	body.home-view .stat-label{margin-bottom:2px;color:#53636a;font-size:9px;letter-spacing:.09em}
	body.home-view .stat-value{color:#142b38;font-size:15px}
	body.home-view .stat-sub{color:#53636a;font-size:10px}
	.terminal.terminal-home{padding:24px 28px 40px;background:#f3ead6;color:#142b38;font:14px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;white-space:normal}
	.home-workbench{width:100%%;max-width:1120px;margin:0 auto}
	.home-header{display:flex;align-items:flex-end;justify-content:space-between;gap:24px;margin:2px 0 22px}
	.home-title{color:#142b38;font:650 30px/1.15 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;letter-spacing:-.025em}
	.home-subtitle{max-width:62ch;margin-top:8px;color:#435760;font:14px/1.55 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-secondary-action{flex:0 0 auto;min-height:40px;padding:9px 12px;border:1px solid #aebdb8;border-radius:6px;background:#fffdf8;color:#142b38;font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer}
	.home-secondary-action:hover,.home-secondary-action:focus-visible{border-color:#e85d36;background:#fff8f1;outline:2px solid #e85d36;outline-offset:2px}
	.home-route{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:6px 18px;margin:0 0 18px;padding:16px 18px;border:1px solid #ccd6d1;border-radius:8px;background:#fffdf8}
	.home-route-label{grid-column:1;color:#53636a;font:600 10px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;letter-spacing:.08em;text-transform:uppercase}
	.home-route-value{grid-column:1;min-width:0;color:#142b38;font:650 16px/1.35 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;overflow-wrap:anywhere}
	.home-route-detail{grid-column:1;color:#435760;font:12px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-route-action{grid-column:2;grid-row:1/4;min-height:40px;padding:8px 12px;border:1px solid #aebdb8;border-radius:6px;background:#fffdf8;color:#142b38;font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;cursor:pointer;white-space:nowrap}
	.home-route-action:hover,.home-route-action:focus-visible{border-color:#e85d36;background:#fff8f1;outline:2px solid #e85d36;outline-offset:2px}
	.home-request{margin:0 0 18px;padding:18px;border:1px solid #ccd6d1;border-radius:8px;background:#fffdf8}
	.home-section-title{color:#142b38;font:650 15px/1.35 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-request-hint{max-width:70ch;margin:5px 0 12px;color:#53636a;font:12px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.term-input-bar.home-composer{display:flex;gap:10px;padding:0;border:0;background:transparent}
	.term-input-bar.home-composer .term-prompt,.term-input-bar.home-composer .term-view-label{display:none}
	.term-input-bar.home-composer .term-input{min-width:0;min-height:44px;max-height:180px;padding:11px 12px;border-color:#aebdb8;border-radius:6px;background:#fff;color:#142b38;font:14px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;resize:vertical}
	.term-input-bar.home-composer .term-input:focus{border-color:#e85d36;box-shadow:0 0 0 3px rgba(232,93,54,.16)}
	.term-input-bar.home-composer .term-input:disabled{border-color:#c5ceca;background:#e4e7e1;color:#53636a;cursor:not-allowed;opacity:1}
	.term-input-bar.home-composer .term-input::placeholder{color:#53636a;opacity:1}
	.term-input-bar.home-composer .term-submit{min-width:132px;min-height:44px;border-color:#142b38;border-radius:6px;background:#142b38;color:#fffdf8;font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.term-input-bar.home-composer .term-submit:hover:not(:disabled){background:#203d4b;filter:none}
	.term-input-bar.home-composer .term-submit:focus-visible{outline:3px solid #e85d36;outline-offset:2px}
	.term-input-bar.home-composer .term-submit:disabled{border-color:#c5ceca;background:#e4e7e1;color:#53636a}
	.term-input-bar.home-composer .term-cancel{min-height:44px;border-radius:6px;border-color:#aebdb8;background:#fffdf8;color:#142b38;font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.term-input-bar.home-composer .term-cancel:hover,.term-input-bar.home-composer .term-cancel:focus-visible{border-color:#b5452d;color:#8f3525;outline:2px solid #b5452d;outline-offset:2px}
	.home-results{min-height:112px;padding:18px;border:1px solid #ccd6d1;border-radius:8px;background:#fffdf8;color:#142b38}
	.home-results-title{margin-bottom:10px;color:#142b38;font:650 15px/1.35 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-result-empty,.home-result-pending{color:#53636a;font:13px/1.55 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-result-summary{max-width:72ch;color:#142b38;font:14px/1.65 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-request-echo{margin-bottom:10px;padding-bottom:9px;border-bottom:1px solid #d7ded8;color:#53636a;font:12px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;overflow-wrap:anywhere}
	.home-result-findings{display:grid;gap:8px;margin:12px 0 0;padding-left:20px;color:#263f49;font:13px/1.55 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-result-note{margin-top:10px;color:#53636a;font:12px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-result-error{padding:10px 12px;border:1px solid #d8a49a;border-radius:5px;background:#fff5f2;color:#852f22;font:13px/1.55 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-receipt{margin-top:14px;border-top:1px solid #d7ded8;padding-top:10px}
	.home-receipt summary{width:max-content;max-width:100%%;color:#203d4b;cursor:pointer;font:600 12px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-receipt summary:focus-visible,.home-more summary:focus-visible{outline:2px solid #e85d36;outline-offset:3px}
	.home-receipt-grid{display:grid;grid-template-columns:minmax(120px,190px) minmax(0,1fr);gap:7px 16px;margin-top:10px;font:12px/1.45 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-receipt-key{color:#53636a;font-weight:600}
	.home-receipt-value{min-width:0;color:#142b38;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}
	.home-tools{max-width:1120px;margin:20px auto 0}
	.home-more{border-top:1px solid #ccd6d1}
	.home-more summary{width:max-content;padding:13px 0;color:#203d4b;cursor:pointer;font:600 12px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-action-grid{display:flex;flex-direction:column}
	.home-action{display:flex;width:100%%;align-items:baseline;gap:14px;padding:10px 0;border:0;border-bottom:1px solid #d7ded8;background:transparent;color:#142b38;text-align:left;cursor:pointer;font:inherit}
	.home-action:hover .home-action-name,.home-action:focus-visible .home-action-name{color:#a83d23}
	.home-action:focus-visible{outline:2px solid #e85d36;outline-offset:2px}
	.home-action-name{color:#142b38;font:650 13px/1.3 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	.home-action-copy{color:#53636a;font:12px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
	@media (min-width:761px) and (max-width:1024px){.terminal.terminal-home{padding:20px}.home-header{align-items:flex-start}}
	.t-sep{border-top:1px solid color-mix(in srgb, var(--gold) 6%%, transparent);margin:6px 0}
	@media (prefers-reduced-motion:reduce){*,*::before,*::after{animation-duration:.01ms!important;animation-iteration-count:1!important;scroll-behavior:auto!important;transition-duration:.01ms!important}}
	@media (prefers-contrast:more){.t-action:focus-visible,.nav-item:focus-visible,.stat-go:focus-visible{outline-width:3px}.t-dim,.t-action{color:var(--ink2)}}
	@media (min-width:761px) and (max-width:1024px){.home-route{grid-template-columns:95px minmax(0,1fr) auto}.home-route-detail{grid-column:2/4}}
	@media (max-width:760px){
	 body{display:block;min-height:100vh;overflow:auto}
	 .sidebar{position:sticky;top:0;width:100%%;min-height:0;height:auto;flex-direction:row;align-items:center;border-right:0;border-bottom:1px solid var(--line)}
	 .sidebar-brand{flex:0 0 auto;padding:12px 14px;border-right:1px solid var(--line);border-bottom:0}
	 .sidebar-nav{display:flex;min-width:0;overflow-x:auto;padding:0;scrollbar-width:none}
	 .sidebar-nav::-webkit-scrollbar{display:none}
	 .nav-item{flex:0 0 auto;padding:12px 10px;border-left:0;border-bottom:2px solid transparent;font-size:11px}
	 .nav-item.active{border-left:0;border-bottom-color:var(--gold)}
	 .nav-tools{position:static;flex:0 0 auto;margin:0;border-top:0;border-left:1px solid var(--line);padding:0}
	 .nav-tools-toggle{min-height:42px;padding:12px 10px;font-size:11px;white-space:nowrap}
	 .nav-tools-toggle::after{margin-left:6px}
	 .nav-tools-panel{flex-direction:row}
	 .sidebar-footer{display:none}
	 .main{margin-left:0;min-height:calc(100vh - 48px);height:auto;align-items:stretch}
	 .main-inner{max-width:none;min-height:calc(100vh - 48px);height:auto;overflow:visible;border-left:0;border-right:0}
	 .stats-bar{display:grid;grid-template-columns:repeat(2,minmax(0,1fr))}
	 .stat{min-width:0;padding:10px 12px}
	 .stat-value{font-size:14px}
	 .stat-sub{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
	 .terminal-wrap{min-height:calc(100vh - 150px);overflow:visible}
	 .term-input{min-width:0;font-size:12px}
	 .term-view-label{display:none}
	 .terminal{min-height:calc(100vh - 150px);overflow-x:hidden;padding:14px 12px;font-size:12px}
	 .t-cmd{display:grid;grid-template-columns:minmax(68px,max-content) minmax(0,1fr);gap:8px;align-items:start}
	 .t-cmd-name{min-width:0}
	 .t-cmd-desc{min-width:0;overflow-wrap:anywhere}
	 .t-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px}
	 .t-col-r{min-width:0;text-align:left}
	 .guard-summary{grid-template-columns:repeat(2,minmax(0,1fr))}
	 .guard-score-card{grid-column:1/-1}
	 .guard-finding{grid-template-columns:22px minmax(0,1fr)}
	 .guard-finding-message{grid-column:2}
	 .engine-grid{grid-template-columns:1fr}
	 .terminal.terminal-home{padding:16px 12px 28px}
	 body.home-view .stats-bar{padding:8px 6px}
	 body.home-view .stat{padding:6px 8px}
	 body.home-view .stat-label{font-size:8px;letter-spacing:.04em}
	 body.home-view .stat-value{font-size:13px}
	 body.home-view .stat-sub{font-size:9px}
	 .home-header{align-items:flex-start;flex-direction:column;gap:12px;margin-bottom:16px}
	 .home-title{font-size:24px}
	 .home-subtitle{font-size:13px}
	 .home-route{grid-template-columns:minmax(0,1fr);gap:6px;padding:14px}
	 .home-route-label,.home-route-value,.home-route-detail,.home-route-action{grid-column:1;grid-row:auto}
	 .home-route-action{justify-self:start;margin-top:5px}
	 .home-request{padding:14px}
	 .term-input-bar.home-composer{flex-wrap:wrap}
	 .term-input-bar.home-composer .term-input{flex:1 1 100%%}
	 .term-submit,.term-cancel{flex:1 1 auto}
	 .home-results{padding:14px}
	 .home-receipt-grid{grid-template-columns:minmax(0,1fr);gap:3px}
	 .home-receipt-value{margin-bottom:6px}
	 .home-action{align-items:flex-start;flex-direction:column;gap:3px}
	}
</style>
</head>
<body>
<a class="skip-link" href="#main-content">Skip to main content</a>
<div class="sidebar">
 <div class="sidebar-brand"><h1>Sirsi Pantheon</h1><div class="sidebar-build" id="build-identity" role="status" aria-live="polite">Reading running build…</div></div>
 <nav class="sidebar-nav" aria-label="Primary navigation">%s</nav>
 <div class="sidebar-footer">LOCAL NODE • 127.0.0.1:%d</div>
</div>
<main class="main" id="main-content" tabindex="-1"><div class="sr-only" id="view-announcement" role="status" aria-live="polite" aria-atomic="true"></div><div class="main-inner">%s</div></main>
</body>
</html>`,
		title,
		brand.CSSVars(brand.Dark),
		ColorBg, ColorWhite,
		ColorBorder, ColorBorder,
		ColorEmerald,
		ColorDim, ColorWhite, ColorEmerald, ColorEmerald,
		ColorDim, ColorWhite,
		ColorBorder,
		ColorBorder,
		ColorEmerald, ColorEmerald,
		ColorBorder, ColorEmerald, ColorWhite,
		navHTML.String(),
		port,
		bodyContent,
	)
}

// ── SPA Entry Point ───────────────────────────────────────────────────

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if token := s.sneAccess.snapshot(); token != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     sneLocalSessionCookie,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
	if r.URL.Path != "/" && r.URL.Path != "/sne" {
		http.NotFound(w, r)
		return
	}

	body := `

<!-- Stats bar -->
<div class="stats-bar">
 <div class="stat stat-go" data-cmd="guard" tabindex="0" role="button"
  title="Open Guard — the view that explains memory pressure and can act on it"><div class="stat-label">RAM</div>
  <div class="stat-value" id="ram-val">—</div>
  <div class="stat-sub" id="ram-label"></div></div>
 <div class="stat"><div class="stat-label">Git</div>
  <div class="stat-value" id="git-val">—</div>
  <div class="stat-sub" id="git-label"></div></div>
 <div class="stat"><div class="stat-label">Components</div>
  <div class="stat-value" id="deity-val">0</div>
  <div class="stat-sub" id="deity-label"></div></div>
 <div class="stat stat-go" data-cmd="hardware" tabindex="0" role="button"
  title="Run hardware detection — CPU / GPU / ANE"><div class="stat-label">Platform</div>
  <div class="stat-value" id="accel-val">—</div>
  <div class="stat-sub" id="accel-label"></div></div>
</div>

<!-- Terminal -->
<div class="terminal-wrap">
 <div class="term-input-bar" id="term-input-bar">
  <span class="term-prompt">&gt;</span>
  <label class="sr-only" for="term-input">Ask a question about this machine or enter a Pantheon command</label>
  <textarea class="term-input" id="term-input" aria-label="Ask a question about this machine or enter a Pantheon command" placeholder="Ask about this machine, or enter a command" autocomplete="off" rows="1"></textarea>
  <button type="button" class="term-submit" id="term-submit" aria-label="Submit the question or command" aria-controls="terminal" disabled>Submit</button>
  <button type="button" class="term-cancel" id="ask-cancel" aria-label="Cancel the current engine request" hidden disabled>Cancel</button>
  <span class="term-view-label" id="view-label">home</span>
 </div>
 <div id="term-input-anchor" hidden></div>
 <div class="terminal" id="terminal" role="log" aria-label="Pantheon command and engine results" aria-live="polite" aria-relevant="additions text" aria-atomic="false">
  <div class="t-line t-dim">Sirsi Pantheon — use the sidebar or type a command</div>
 </div>
</div>

<script>
(function(){
'use strict';
const T=document.getElementById('terminal');
const fmtSize=b=>{if(b>=1073741824)return(b/1073741824).toFixed(1)+' GB';
 if(b>=1048576)return(b/1048576).toFixed(1)+' MB';if(b>=1024)return(b/1024).toFixed(1)+' KB';return b+' B'};
const ago=ts=>{if(!ts)return'';const d=Date.now()-new Date(ts).getTime();
 if(!Number.isFinite(d))return'time unavailable';if(d<0)return'timestamp ahead of local clock';
 if(d<60e3)return Math.floor(d/1e3)+'s ago';if(d<3600e3)return Math.floor(d/6e4)+'m ago';
 if(d<864e5)return Math.floor(d/36e5)+'h ago';return Math.floor(d/864e5)+'d ago'};
const viewLoaders={home:viewHome,fleet:viewFleet,scan:viewScan,ghosts:viewGhosts,guard:viewGuard,
 notifications:viewNotifications,horus:viewHorus,vault:viewVault,engine:viewEngine,sne:viewSNE,recovery:viewRecovery,ra:viewRa};
const viewAnnouncements={home:'Home workspace opened.',fleet:'Fleet view opened.',scan:'Scan view opened.',
 ghosts:'Ghosts view opened.',guard:'Guard view opened.',notifications:'Notifications view opened.',
 horus:'Code Graph view opened.',vault:'Vault view opened.',engine:'Engine view opened.',
 sne:'SNE view opened.',recovery:'Recovery view opened.',ra:'Ra view opened.'};
function viewFromLocation(){
 if(location.pathname==='/sne')return'sne';
 const requested=new URLSearchParams(location.search).get('view');
 return Object.prototype.hasOwnProperty.call(viewLoaders,requested)?requested:'home';
}
let currentView=viewFromLocation();let running=false;let selectionPending=false;let engineViewRequest=0;let homeRouteRequest=0;
const workerControlLeases=Object.create(null);

function out(text,cls){const d=document.createElement('div');d.className='t-line '+(cls||'t-out');
 d.textContent=text;T.appendChild(d);if(T.children.length>800)T.removeChild(T.firstChild);T.scrollTop=T.scrollHeight}
function sep(){const d=document.createElement('div');d.className='t-sep';T.appendChild(d)}
function clear(){T.textContent=''}

/* Stats polling */
function renderStats(s){if(!s)return;
 /* Body is a Sprintf ARGUMENT, not format — a literal % is correct here. */
 document.getElementById('ram-val').textContent=(s.ram_icon||'')+' '+Math.round(s.ram_percent||0)+'%';
 /* Show real used/total when the producer supplies them; never render
    fabricated numbers when they're absent (data honesty). */
 document.getElementById('ram-label').textContent=(s.total_ram>0
  ?fmtSize(s.used_ram||0)+' / '+fmtSize(s.total_ram)+' · ':'')+(s.ram_pressure||'');
 document.getElementById('git-val').textContent=(s.uncommitted_files||0)+' dirty';
 document.getElementById('git-label').textContent=s.git_branch||'';
		 const componentsKnown=s.components_known===true&&Number.isInteger(s.component_count);
		 document.getElementById('deity-val').textContent=componentsKnown?String(s.component_count):'—';
		 document.getElementById('deity-label').textContent=componentsKnown?((s.components||[]).join(', ')||'None detected'):'Process inventory unavailable';
 document.getElementById('accel-val').textContent=s.accel_icon||'';
 document.getElementById('accel-label').textContent=s.primary_accelerator||''}
function pollStats(){fetch('/api/stats').then(r=>r.json()).then(renderStats).catch(function(){})}
pollStats();setInterval(pollStats,10000);

/* Show the identity of the process serving this page, not just its URL. */
const buildLabel=document.getElementById('build-identity');
fetch('/api/identity').then(function(r){if(!r.ok)throw new Error('HTTP '+r.status);return r.json()}).then(function(data){
 if(data.schema!=='pantheon.dashboard-identity/v1')throw new Error('unsupported identity schema');
 const info=data;
 const version=String(info.version||'unknown');
 const commit=String(info.commit||'unknown');
 const binary=String(info.binary||'sirsi');
 buildLabel.textContent='Build · '+version+' · '+commit.slice(0,10)+(info.dirty?' · modified':'');
 buildLabel.title='Running process: '+binary+' · '+version+' · '+commit+(info.dirty?' · modified source':'');
 buildLabel.dataset.state='ready';
}).catch(function(){
 buildLabel.textContent='Build identity unavailable — verify this running app before continuing.';
 buildLabel.title='This running dashboard did not provide a readable Pantheon build identity. Verify the app before continuing.';
 buildLabel.dataset.state='unavailable';
});

/* ── View system ──────────────────────────────────────── */
window.switchView=function(view,options){
 options=options||{};
 if(!Object.prototype.hasOwnProperty.call(viewLoaders,view))view='home';
 const changedView=currentView!==view;
 const nextURL=view==='home'?'/':'/?view='+encodeURIComponent(view);
 if(options.history!==false&&location.pathname+location.search!==nextURL)history.pushState({view:view},'',nextURL);
 if(view!=='home'){
  const bar=document.getElementById('term-input-bar');
  const anchor=document.getElementById('term-input-anchor');
  if(bar&&anchor&&bar.parentElement!==anchor.parentElement)anchor.before(bar);
  if(bar)bar.classList.remove('home-composer');
 }
 currentView=view;
 document.body.classList.toggle('home-view',view==='home');
 T.classList.toggle('terminal-home',view==='home');
 T.setAttribute('role',view==='home'?'region':'log');
 T.setAttribute('aria-label',view==='home'?'Pantheon request workspace':'Pantheon command and engine results');
 T.setAttribute('aria-live',view==='home'?'off':'polite');
 document.getElementById('view-label').textContent=view;
 if(changedView)document.getElementById('view-announcement').textContent=viewAnnouncements[view];
 const toolsNav=document.querySelector('.nav-tools');
 if(toolsNav)toolsNav.open=['scan','ghosts','guard','notifications','horus','vault','recovery'].indexOf(view)!==-1;
 document.querySelectorAll('.nav-item').forEach(function(n){
  const active=n.dataset.view===view;
  n.classList.toggle('active',active);
  if(active)n.setAttribute('aria-current','page');else n.removeAttribute('aria-current')});
 clear();
 viewLoaders[view]();
 if(changedView)document.getElementById('main-content').focus({preventScroll:true});
};
window.addEventListener('popstate',function(){switchView(viewFromLocation(),{history:false})});
document.querySelectorAll('.nav-item[data-view]').forEach(function(link){
 link.addEventListener('click',function(event){
  if(event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
  event.preventDefault();switchView(link.dataset.view);
 });
});

function viewHome(){
 clear();
 setPromptRouteReady(false);
 document.body.classList.add('home-view');T.classList.add('terminal-home');
 T.setAttribute('role','region');T.setAttribute('aria-label','Pantheon request workspace');T.setAttribute('aria-live','off');
 const workbench=document.createElement('div');workbench.className='home-workbench';
 const header=document.createElement('header');header.className='home-header';
 const copy=document.createElement('div');
 const title=document.createElement('h1');title.className='home-title';title.id='home-title';title.textContent='Work with Sirsi Pantheon';copy.appendChild(title);
 const subtitle=document.createElement('p');subtitle.className='home-subtitle';subtitle.textContent='Ask about this machine’s current diagnostics. Pantheon uses the selected route and returns the answer with its route evidence.';copy.appendChild(subtitle);
 header.appendChild(copy);
 const fleet=document.createElement('button');fleet.type='button';fleet.className='home-secondary-action';fleet.textContent='View worker state';fleet.addEventListener('click',function(){switchView('fleet')});header.appendChild(fleet);
 workbench.appendChild(header);

 const route=document.createElement('section');route.className='home-route';route.setAttribute('aria-live','polite');route.setAttribute('aria-label','Current engine route policy');
 const routeLabel=document.createElement('span');routeLabel.className='home-route-label';routeLabel.textContent='Current route policy';route.appendChild(routeLabel);
 const routeValue=document.createElement('span');routeValue.className='home-route-value';routeValue.textContent='Reading configured route…';route.appendChild(routeValue);
 const routeDetail=document.createElement('span');routeDetail.className='home-route-detail';routeDetail.textContent='Availability and session identity are confirmed when a request runs.';route.appendChild(routeDetail);
 const routeAction=document.createElement('button');routeAction.className='home-route-action';routeAction.type='button';routeAction.textContent='Change route';routeAction.addEventListener('click',function(){switchView('engine')});route.appendChild(routeAction);
 workbench.appendChild(route);

	const request=document.createElement('section');request.className='home-request';request.setAttribute('aria-labelledby','home-request-title');
	const requestTitle=document.createElement('h2');requestTitle.className='home-section-title';requestTitle.id='home-request-title';requestTitle.textContent='Ask a question';request.appendChild(requestTitle);
	const requestHint=document.createElement('p');requestHint.className='home-request-hint';requestHint.textContent='Checking the configured route before enabling prompt submission…';request.appendChild(requestHint);
	const commandBar=document.getElementById('term-input-bar');
	if(commandBar){commandBar.classList.add('home-composer');request.appendChild(commandBar)}
	workbench.appendChild(request);

	const results=document.createElement('section');results.className='home-results';results.setAttribute('aria-labelledby','home-results-title');
	const resultsTitle=document.createElement('h2');resultsTitle.className='home-results-title';resultsTitle.id='home-results-title';resultsTitle.textContent='Response and route evidence';results.appendChild(resultsTitle);
	const content=document.createElement('div');content.id='home-result-content';content.setAttribute('aria-live','polite');content.setAttribute('aria-atomic','true');
 const empty=document.createElement('p');empty.className='home-result-empty';empty.textContent='Submit a question to see the returned findings and request receipt here.';content.appendChild(empty);
 results.appendChild(content);workbench.appendChild(results);T.appendChild(workbench);

 const routeRequest=++homeRouteRequest;
 fetch('/api/engine').then(function(r){return readJSONResponse(r,'Engine policy')}).then(function(data){
  if(currentView!=='home'||routeRequest!==homeRouteRequest)return;
  const selected=(data.connectors||[]).find(function(connector){return connector.kind===data.preferred&&connector.variant===data.preferred_variant});
  const routeID=data.preferred?(String(data.preferred).toUpperCase()+(data.preferred_variant?' · '+data.preferred_variant:'')):'';
  routeValue.textContent=selected?engineDisplayName(selected):(routeID||'No route selected');
  if(!selected){
   routeDetail.textContent=data.preferred?'The selected route is not present in the configured connector list.':'No route is selected.';
   requestHint.textContent='Choose a configured SNE, MLX, or oMLX route before submitting a prompt.';
   setPromptRouteReady(false);
   return;
  }
  const boundary=selected&&selected.data_boundary==='on-device'?'a loopback endpoint':selected&&selected.data_boundary==='remote'?'a non-loopback endpoint':'not disclosed';
  const fallbackNote=data.allow_fallback?' Fallback is enabled and may change the destination; inspect the receipt.':'';
  routeDetail.textContent='Configured route: '+routeID+'. Endpoint boundary: '+boundary+'. Availability is confirmed when the session opens.'+fallbackNote;
  requestHint.textContent='Questions are grounded in live system diagnostics. Press Enter to submit or Shift+Enter for a new line.';
  setPromptRouteReady(true);
 }).catch(function(e){if(currentView!=='home'||routeRequest!==homeRouteRequest)return;routeValue.textContent='Unavailable';routeDetail.textContent='Engine policy could not be read: '+e.message;requestHint.textContent='Prompt submission is unavailable until Pantheon can confirm a configured route. Open Engine to retry or select a route.';setPromptRouteReady(false)});
	const tools=document.createElement('section');tools.className='home-tools';tools.setAttribute('aria-label','Additional Pantheon tools');
	const more=document.createElement('details');more.className='home-more';
	const moreSummary=document.createElement('summary');moreSummary.textContent='Other system tools';more.appendChild(moreSummary);
	const secondary=document.createElement('div');secondary.className='home-action-grid';
	[['doctor','Check system health','Review this machine’s current health findings.'],['scan','Scan infrastructure','Find infrastructure waste and residual resources.'],['guard','Review system controls','Inspect guard status and available controls.'],['ghosts','Find application remnants','Review unused application files.'],['network','Audit network security','Review available network findings.'],['hardware','Inspect hardware','View CPU, GPU, and ANE detection.'],['quality','Check code governance','Review repository governance findings.'],['dedup','Find duplicate files','Review duplicate-file findings.']].forEach(function(item){secondary.appendChild(makeHomeAction(item))});
	more.appendChild(secondary);tools.appendChild(more);T.appendChild(tools);
}

function makeHomeAction(item){
 const button=document.createElement('button');button.type='button';button.className='home-action';
 const name=document.createElement('span');name.className='home-action-name';name.textContent=item[1];button.appendChild(name);
 const detail=document.createElement('span');detail.className='home-action-copy';detail.textContent=item[2];button.appendChild(detail);
 button.addEventListener('click',function(){input.value='';exec(item[0])});
 return button;
}

function engineRouteID(connector){return connector.kind.toUpperCase()+' · '+connector.variant}
function engineDisplayName(connector){return connector.display_name||engineRouteID(connector)}
function readJSONResponse(response,label){return response.text().then(function(text){let body=null;if(text){try{body=JSON.parse(text)}catch(e){if(response.ok)throw new Error(label+' response was not valid JSON')}}if(!response.ok){const detail=body&&typeof body.error==='string'&&body.error.trim()?body.error:label+' request returned HTTP '+response.status;throw new Error(detail)}if(!body||typeof body!=='object'||Array.isArray(body))throw new Error(label+' response was not a JSON object');return body})}

function appendEnginePromptAction(){
 const row=document.createElement('div');row.className='t-line engine-followup';
 const button=document.createElement('button');button.type='button';button.className='engine-followup-button';
 button.textContent='Ask a question with this route';
 button.addEventListener('click',function(){switchView('home');input.focus()});
 row.appendChild(button);T.appendChild(row);button.focus();T.scrollTop=T.scrollHeight;
}

function viewEngine(){
 const requestID=++engineViewRequest;
 out('Engine — Pantheon routing','t-gold');
 out('Choose the preferred route for this dashboard. Dashboard prompts use the shared Engine ABI; CLI choices are explicit per invocation.','t-dim');
 sep();
 fetch('/api/engine').then(function(r){return readJSONResponse(r,'Engine policy')}).then(function(data){
  if(currentView!=='engine'||requestID!==engineViewRequest)return;
  out('Preferred    '+(data.preferred||'none')+(data.preferred_variant?' · '+data.preferred_variant:''),'t-ok');
  out('Fallback     '+(data.allow_fallback?'explicitly allowed':'disabled'),'t-out');
  out('Configured engines','t-head');
  const grid=document.createElement('div');grid.className='engine-grid';
  (data.connectors||[]).forEach(function(connector){
   const selected=connector.kind===data.preferred&&connector.variant===data.preferred_variant;
   const card=document.createElement('section');card.className='engine-card'+(selected?' selected':'');
   const displayName=engineDisplayName(connector);
   card.setAttribute('aria-label',displayName+' route; '+engineRouteID(connector));
   const label=document.createElement('div');label.className='engine-name';label.textContent=displayName;
   const status=document.createElement('div');status.className='engine-status';status.textContent=(selected?'Preferred policy':'Configured')+' · '+engineRouteID(connector);
   const caps=connector.capabilities||{};const names=[];
   Object.keys(caps).forEach(function(k){if(caps[k]===true)names.push(k)});
   const contextual=(connector.contextual_capabilities||[]).map(function(k){return k+' · live check before use'});
   const detail=document.createElement('div');detail.className='engine-caps';detail.textContent=names.concat(contextual).join(' · ')||'No optional capabilities';
   const boundary=document.createElement('div');boundary.className='engine-boundary';
   boundary.textContent='Configured endpoint: '+({ 'on-device':'loopback address', 'remote':'non-loopback address' }[connector.data_boundary]||'not disclosed');
   const choose=document.createElement('button');choose.className='engine-select';choose.type='button';choose.textContent=selected?'Selected':selectionPending?'Updating…':'Use '+displayName;
   choose.disabled=selected||selectionPending;choose.setAttribute('aria-pressed',selected?'true':'false');
   choose.setAttribute('aria-label',(selected?'Selected ':'Use ')+displayName+' route '+engineRouteID(connector));
   if(!selected){choose.onclick=function(){selectEngine(connector.kind,connector.variant,choose)}}
   card.appendChild(label);card.appendChild(status);card.appendChild(detail);card.appendChild(boundary);card.appendChild(choose);grid.appendChild(card);
  });
  T.appendChild(grid);
  out('Selection changes policy only; availability is proved when a session opens.','t-dim');
 }).catch(function(e){if(currentView==='engine'&&requestID===engineViewRequest)out('Engine selection unavailable: '+e.message,'t-err')});
}

function selectEngine(kind,variant,button){
 if(selectionPending)return;
 selectionPending=true;syncSubmitControl();
 document.querySelectorAll('.engine-select').forEach(function(control){control.disabled=true});
 if(button){button.textContent='Saving…';button.setAttribute('aria-busy','true')}
 out('Selecting dashboard route…','t-dim');
	fetch('/api/engine/select',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({preferred:kind,preferred_variant:variant})})
	 .then(function(r){return readJSONResponse(r,'Route change')})
 .then(function(snapshot){
 if(snapshot.preferred!==kind||snapshot.preferred_variant!==variant)throw new Error('canonical policy readback did not match the requested route');
  selectionPending=false;syncSubmitControl();
  if(currentView==='engine'){
   switchView('engine');
   out('Dashboard route selected: '+kind.toUpperCase()+' · '+variant+'. Live availability is checked when a session opens.','t-ok');
   appendEnginePromptAction();
  }else if(currentView==='home'){
   viewHome();
  }
 })
 .catch(function(e){
  selectionPending=false;syncSubmitControl();
  if(currentView==='engine'){
   switchView('engine');
   out('Engine selection rejected: '+e.message,'t-err');
  }else if(currentView==='home'){
   viewHome();
  }
 })
 .finally(function(){selectionPending=false;syncSubmitControl()});
}

function viewSNE(){
 out('SNE — Local AI Engine','t-gold');
 out('Pantheon installs, verifies, admits, and supervises. SNE computes. Nexus presents.','t-dim');
 sep();
 fetch('/api/sne').then(function(r){return r.json().then(function(body){
  if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})}).then(function(data){
  out('Service      '+(data.ready?'READY':'NOT READY'),data.ready?'t-ok':'t-err');
  out('Model        '+(data.active_model||'none'),'t-out');
  out('Mac          '+(data.device_family||'unknown'),'t-out');
  out('Memory       '+fmtSize(data.unified_memory_bytes||0),'t-out');
  if(data.runtime_catalog){
   const c=data.runtime_catalog;
   out('Catalog      '+(c.state||'unknown')+(c.signed_required?' · signed':''),c.state==='verified'?'t-ok':'t-err');
   if(c.catalog_id)out('Catalog ID   '+c.catalog_id,'t-out');
   if(c.version_sha256)out('Version      '+c.version_sha256.slice(0,12)+'… · '+c.entries+' entries · '+c.versions+' retained','t-out');
   out('Rollback     '+(c.rollback_available?'available':'not available'),c.rollback_available?'t-gold':'t-dim');
   if(c.update_feed_configured){
    const check=document.createElement('div');check.className='t-line t-action';check.textContent='[Check signed catalog updates]';check.tabIndex=0;check.setAttribute('role','button');
    check.onclick=checkSNECatalogUpdates;check.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();checkSNECatalogUpdates()}};T.appendChild(check);
   }
   (c.retained_versions||[]).forEach(function(version){
    if(version===c.version_sha256)return;
    const row=document.createElement('div');row.className='t-line t-row';
    const label=document.createElement('span');label.className='t-col';label.style.flex='1';label.textContent='Retained     '+version.slice(0,12)+'…';
    const rollback=document.createElement('span');rollback.className='t-action';rollback.textContent='[Rollback]';rollback.tabIndex=0;rollback.setAttribute('role','button');
    rollback.onclick=function(){mutateSNECatalog('rollback',version)};
    rollback.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();mutateSNECatalog('rollback',version)}};
    const remove=document.createElement('span');remove.className='t-action';remove.style.marginLeft='12px';remove.textContent='[Remove]';remove.tabIndex=0;remove.setAttribute('role','button');
    remove.onclick=function(){mutateSNECatalog('remove',version)};
    remove.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();mutateSNECatalog('remove',version)}};
    row.appendChild(label);row.appendChild(rollback);row.appendChild(remove);T.appendChild(row);
   });
   if(c.error)out('Catalog error '+c.error,'t-err');
  }
	  if(data.recovery)out('Next         '+data.recovery,'t-dim');
	  out('Model tools   '+(data.lifecycle_tools_ready?'ready':'unavailable')+(data.lifecycle_tools_status?' · '+data.lifecycle_tools_status:''),data.lifecycle_tools_ready?'t-ok':'t-err');
  sep();
  out('CATALOG','t-head');
  (data.catalog||[]).forEach(function(m){
   const row=document.createElement('div');row.className='t-line t-row';
   const state=document.createElement('span');state.style.width='22px';
   state.textContent=m.active?'●':(m.installed?'◆':'○');
   state.style.color=m.active?'var(--ok)':(m.state==='incompatible-memory'?'var(--danger)':'var(--gold)');
   const name=document.createElement('span');name.className='t-col';name.style.flex='1';
   name.textContent=m.parameter_class+' · '+m.weight_format.toUpperCase()+m.weight_bits+' · '+m.execution_mode+(m.runtime_id?' · '+m.runtime_id:'');
   name.title=m.model_id+' · '+(m.support_status||'unqualified')+(m.next_gate?' · next '+m.next_gate:'');
   const support=document.createElement('span');support.className='t-col-r';support.style.minWidth='118px';
   support.textContent=m.support_status||'unqualified';
   support.style.color=m.support_status==='release-supported'?'var(--ok)':(m.support_status==='research-only'?'var(--danger)':'var(--gold)');
   const mem=document.createElement('span');mem.className='t-col-r';mem.textContent=fmtSize(m.memory_bytes);
   const action=document.createElement('span');action.className='t-action';action.style.marginLeft='12px';
   action.textContent='['+m.action_label+']';
   if(!m.action_enabled){action.style.cursor='default';action.style.opacity='.55';action.style.textDecoration='none';action.setAttribute('aria-disabled','true')}
   else{action.tabIndex=0;action.setAttribute('role','button');action.setAttribute('aria-label',m.action_label+' '+m.model_id+(m.runtime_id?' using '+m.runtime_id:''));action.onclick=function(){actSNE(m)};
    action.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();actSNE(m)}}}
	   row.appendChild(state);row.appendChild(name);row.appendChild(support);row.appendChild(mem);row.appendChild(action);
	   if(m.installed&&!m.active){const remove=document.createElement('span');remove.className='t-action';remove.style.marginLeft='12px';remove.textContent='[Remove model]';
	    if(!m.removal_enabled){remove.style.cursor='default';remove.style.opacity='.55';remove.style.textDecoration='none';remove.setAttribute('aria-disabled','true');remove.title=m.removal_reason||'Stop SNE before removal'}
	    else{remove.tabIndex=0;remove.setAttribute('role','button');remove.setAttribute('aria-label','Remove installed model '+m.model_id);remove.onclick=function(){removeSNEModel(m)};
	     remove.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();removeSNEModel(m)}}}row.appendChild(remove)}
	   T.appendChild(row);
   if(m.license_id){const license=document.createElement('div');license.className='t-line t-dim';license.style.cssText='padding-left:38px;font-size:10px';license.appendChild(document.createTextNode('License: '+m.license_id+(m.license_acceptance_required?' · acceptance required':'')));
    if(m.license_url){const terms=document.createElement('a');terms.href=m.license_url;terms.target='_blank';terms.rel='noopener noreferrer';terms.textContent=' · Review terms';terms.style.marginLeft='4px';license.appendChild(terms)}T.appendChild(license)}
   if(m.reason){const why=document.createElement('div');why.className='t-line t-dim';
    why.style.cssText='padding-left:38px;font-size:10px';why.textContent=m.reason;T.appendChild(why)}
   if(m.next_gate&&m.next_gate!=='complete'){const gate=document.createElement('div');gate.className='t-line t-dim';
    gate.style.cssText='padding-left:38px;font-size:10px';gate.textContent='Next qualification gate: '+m.next_gate;T.appendChild(gate)}
  });
  out('');out('● active · ◆ installed · ○ available · only release-supported tuples can install or start','t-dim');
  if(data.lifecycle&&data.lifecycle.state!=='stopped'&&data.lifecycle.state!=='not-configured'){
   if(data.lifecycle.state==='failed')renderSNELifecycleFailure(data.lifecycle);
   else out('Lifecycle    '+data.lifecycle.state+(data.lifecycle.runtime_id?' · '+data.lifecycle.runtime_id:''),'t-dim');
  }
  out('Installs are transactional; starts are exact-tuple, package-bound, and Pantheon-supervised.','t-dim');
  const activeSupported=(data.catalog||[]).some(function(m){return m.active&&m.support_status==='release-supported'});
  if(data.ready&&activeSupported){const nexus=document.createElement('div');nexus.className='t-line t-action';nexus.textContent='[Open Nexus Local AI]';nexus.tabIndex=0;nexus.setAttribute('role','button');nexus.setAttribute('aria-label','Open Nexus with the verified local SNE model');
   nexus.onclick=openSNENexus;nexus.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();openSNENexus()}};T.appendChild(nexus)}
  const diagnostics=document.createElement('div');diagnostics.className='t-line t-action';diagnostics.textContent='[Export privacy-safe support diagnostics]';diagnostics.tabIndex=0;diagnostics.setAttribute('role','button');
  diagnostics.onclick=exportSNEDiagnostics;diagnostics.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();exportSNEDiagnostics()}};T.appendChild(diagnostics);
  const support=document.createElement('div');support.className='t-line t-action';support.textContent='[Export complete SNE support bundle]';support.tabIndex=0;support.setAttribute('role','button');support.setAttribute('aria-label','Export complete privacy-safe SNE support bundle');
  support.onclick=exportSNESupportBundle;support.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();exportSNESupportBundle()}};T.appendChild(support);
 }).catch(function(e){out('SNE read model unavailable: '+e.message,'t-err')});
}

function viewRecovery(){
 out('↻ Recovery — Applications & Services','t-gold');
 out('Restore resumes declared durable state. Fresh deliberately clears registered transient queues or caches.','t-dim');
 sep();
 fetch('/api/recovery').then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(data){
  const targets=data.targets||[];
  if(!targets.length){out('No governed recovery targets are registered.','t-dim');return}
  targets.forEach(function(target){
   const row=document.createElement('div');row.className='t-line t-row';
   const state=document.createElement('span');state.className='t-col';state.style.flex='1';state.textContent=target.target_id+' · '+target.kind+(target.auto_resume?' · auto-resume':'')+(target.phase?' · '+target.phase:'');row.appendChild(state);
   if(target.restore_supported)row.appendChild(recoveryAction('[Restore]','Restore '+target.target_id+' from its declared durable session or checkpoint?',function(){restartRecoveryTarget(target.target_id,'restore')}));
   if(target.fresh_supported){const fresh=recoveryAction('[Fresh restart]','Fresh restart '+target.target_id+'? Pantheon will discard only its registered transient queue/cache files.',function(){restartRecoveryTarget(target.target_id,'fresh')});fresh.style.marginLeft='12px';row.appendChild(fresh)}
   if(target.phase==='captured'||target.phase==='stopped'||target.phase==='started'){const resume=recoveryAction('[Resume interrupted]','Resume the interrupted '+target.mode+' operation for '+target.target_id+'?',function(){resumeRecoveryTarget(target.target_id)});resume.style.marginLeft='12px';row.appendChild(resume)}
   T.appendChild(row);
   if(target.failure_code)out('  Failure      '+target.failure_code,'t-err');
  });
  out('');out('Every action is registry-bound, receipt-backed, and requires a verified replacement process.','t-dim');
 }).catch(function(e){out('Recovery unavailable: '+e.message,'t-err')});
}

function recoveryAction(label,question,action){
 const control=document.createElement('span');control.className='t-action';control.textContent=label;control.tabIndex=0;control.setAttribute('role','button');control.setAttribute('aria-label',label.replace(/[\[\]]/g,''));
 control.onclick=function(){if(confirm(question))action()};control.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();control.click()}};return control;
}

function restartRecoveryTarget(targetID,mode){
 out((mode==='restore'?'Restoring ':'Fresh restarting ')+targetID+'…','t-gold');
 fetch('/api/recovery/restart',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({target_id:targetID,mode:mode})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.failure_code||body.error||('HTTP '+r.status));return body})})
 .then(function(){out(targetID+' is ready.','t-ok');setTimeout(viewRecovery,300)})
 .catch(function(e){out('Recovery rejected: '+e.message,'t-err');setTimeout(viewRecovery,300)});
}

function resumeRecoveryTarget(targetID){
 out('Resuming interrupted recovery for '+targetID+'…','t-gold');
 fetch('/api/recovery/resume',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({target_id:targetID,mode:''})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.failure_code||body.error||('HTTP '+r.status));return body})})
 .then(function(){out(targetID+' is ready.','t-ok');setTimeout(viewRecovery,300)})
 .catch(function(e){out('Resume rejected: '+e.message,'t-err');setTimeout(viewRecovery,300)});
}

function exportSNEDiagnostics(){
 const link=document.createElement('a');link.href='/api/sne/diagnostics';link.download='';document.body.appendChild(link);link.click();link.remove();
 out('Exported privacy-safe SNE support diagnostics.','t-ok');
}

function openSNENexus(){
 out('Opening Nexus with this Mac\'s private SNE capability…','t-gold');
 fetch('/api/sne/nexus/open',{method:'POST'}).then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error((body.error&&body.error.message)||body.error||('HTTP '+r.status));return body})})
 .then(function(body){out('Nexus opened for '+body.model+'.','t-ok')})
 .catch(function(e){out('Nexus handoff rejected: '+e.message,'t-err')});
}

function exportSNESupportBundle(){
 if(!confirm('Create a privacy-safe SNE support bundle? It includes package identity, admission and resource state, local health counters, and signature status. It excludes conversations, model data, caches, logs, environment values, network configuration, and machine identifiers. Review the archive before sharing.'))return;
 out('Creating privacy-safe SNE support bundle…','t-gold');
 fetch('/api/sne/support-bundle',{method:'POST'}).then(function(r){if(!r.ok)return r.json().then(function(body){throw new Error(body.error||('HTTP '+r.status))});return r.blob()})
 .then(function(blob){const url=URL.createObjectURL(blob);const link=document.createElement('a');link.href=url;link.download='sirsi-sne-support.zip';document.body.appendChild(link);link.click();link.remove();setTimeout(function(){URL.revokeObjectURL(url)},1000);out('Exported privacy-verified SNE support bundle. Review it before sharing.','t-ok')})
 .catch(function(e){out('Support bundle export failed: '+e.message,'t-err')});
}

function renderSNELifecycleFailure(state){
 out('Lifecycle    failed'+(state.error_code?' · '+state.error_code:'')+(state.error?' · '+state.error:''),'t-err');
 const resource=state.resource_admission;
 if(resource){
  out('Memory       '+fmtSize(resource.required_bytes)+' required · '+fmtSize(resource.available_ram_bytes)+' available','t-dim');
  out('Headroom     '+fmtSize(resource.dynamic_reserve_bytes)+' dynamic · '+fmtSize(resource.lifecycle_reserve_bytes)+' lifecycle','t-dim');
  out('Swap         '+fmtSize(resource.swap_used_bytes)+' used · '+fmtSize(resource.swap_limit_bytes)+' limit','t-dim');
 }
 if(state.recovery)out('Recovery     '+state.recovery,'t-gold');
 if(state.model_id){
  const retry=document.createElement('div');retry.className='t-line t-action';retry.textContent='[Retry when conditions are safe]';retry.tabIndex=0;retry.setAttribute('role','button');
  retry.onclick=function(){startSNE(state.model_id,state.runtime_id||'')};
  retry.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();startSNE(state.model_id,state.runtime_id||'')}};
  T.appendChild(retry);
 }
}

function installSNE(catalogEntry,modelID,licenseID,licenseURL){
 if(!licenseID||!licenseURL){out('Install blocked: verified license terms are unavailable.','t-err');return}
 const dialog=document.createElement('dialog');dialog.setAttribute('aria-labelledby','sne-license-title');dialog.style.cssText='max-width:560px;border:1px solid var(--gold);background:var(--bg);color:var(--text);padding:22px;box-shadow:0 18px 60px rgba(0,0,0,.45)';
 const title=document.createElement('h2');title.id='sne-license-title';title.textContent='Review model terms';title.style.cssText='margin:0 0 12px;color:var(--gold);font-size:16px';dialog.appendChild(title);
 const identity=document.createElement('p');identity.textContent=modelID;identity.style.cssText='overflow-wrap:anywhere';dialog.appendChild(identity);
 const explanation=document.createElement('p');explanation.textContent='Pantheon will download and verify this exact signed model tuple. Acceptance is recorded in its checkout receipt.';dialog.appendChild(explanation);
 const terms=document.createElement('a');terms.href=licenseURL;terms.target='_blank';terms.rel='noopener noreferrer';terms.textContent='Review '+licenseID+' in a new window';terms.style.color='var(--gold)';dialog.appendChild(terms);
 const consentRow=document.createElement('label');consentRow.style.cssText='display:flex;gap:10px;align-items:flex-start;margin:20px 0';
 const consent=document.createElement('input');consent.type='checkbox';consent.setAttribute('aria-describedby','sne-license-consent-copy');
 const consentCopy=document.createElement('span');consentCopy.id='sne-license-consent-copy';consentCopy.textContent='I reviewed and accept these terms for this model installation.';consentRow.appendChild(consent);consentRow.appendChild(consentCopy);dialog.appendChild(consentRow);
 const actions=document.createElement('div');actions.style.cssText='display:flex;justify-content:flex-end;gap:10px';
 const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
 const install=document.createElement('button');install.type='button';install.textContent='Accept and install';install.disabled=true;
 consent.onchange=function(){install.disabled=!consent.checked};cancel.onclick=function(){dialog.close('cancel')};
 install.onclick=function(){if(!consent.checked)return;install.disabled=true;cancel.disabled=true;dialog.close('accepted');beginSNEInstall(catalogEntry)};
 actions.appendChild(cancel);actions.appendChild(install);dialog.appendChild(actions);dialog.addEventListener('close',function(){dialog.remove()});document.body.appendChild(dialog);dialog.showModal();consent.focus();
}

function beginSNEInstall(catalogEntry){
 out('Starting verified installation for '+catalogEntry+'…','t-gold');
 fetch('/api/sne/install',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({catalog_entry:catalogEntry,accept_license:true,allow_research:false})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(job){pollSNEInstall(job.id)}).catch(function(e){out('Install rejected: '+e.message,'t-err')});
}

function discardSNEPrepared(catalogEntry,modelID){
 if(!confirm('Discard the retained download for '+modelID+'? This removes only the failed prepared source. Installed models and shared model-store objects are not changed.'))return;
 out('Discarding retained download for '+catalogEntry+'…','t-gold');
 fetch('/api/sne/prepared/discard',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({catalog_entry:catalogEntry})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(body){const result=body.result||{};out('Discarded retained download for '+result.catalog_entry+' · revision '+result.revision+'. Installed models were not changed.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Retained download cleanup rejected: '+e.message,'t-err')});
}

function actSNE(model){
 if(model.action_kind==='install'){installSNE(model.catalog_entry,model.model_id,model.license_id,model.license_url);return}
 if(model.action_kind==='start'){startSNE(model.model_id,model.runtime_id||'');return}
 if(model.action_kind==='stop'){stopSNE();return}
}

function removeSNEModel(model){
	 if(!model.removal_enabled){out(model.removal_reason||'Stop SNE before removing this model.','t-err');return}
	 if(!confirm('Remove '+model.model_id+' from this Mac? Pantheon will remove its governed model view, retain any objects shared by another installed model, and allow the model to be installed again later.'))return;
	 out('Removing '+model.model_id+' transactionally…','t-gold');
	 fetch('/api/sne/remove',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({catalog_entry:model.catalog_entry,model_id:model.model_id})})
	 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
	 .then(function(body){const result=body.result||{};out('Removed '+model.model_id+'. Shared objects retained: '+(result.objects_retained||0)+'.','t-ok');setTimeout(viewSNE,300)})
	 .catch(function(e){out('Removal rejected: '+e.message,'t-err')});
}

function startSNE(modelID,runtimeID){
 out('Starting verified runtime for '+modelID+(runtimeID?' · '+runtimeID:'')+'…','t-gold');
 fetch('/api/sne/start',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({model_id:modelID,runtime_id:runtimeID||undefined})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){pollSNELifecycle()}).catch(function(e){out('Start rejected: '+e.message,'t-err')});
}

function stopSNE(){
 out('Stopping SNE under Pantheon supervision…','t-gold');
 fetch('/api/sne/stop',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){out('SNE stopped.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Stop rejected: '+e.message,'t-err')});
}

function mutateSNECatalog(action,version){
 const verb=action==='rollback'?'Roll back to':'Remove inactive';
 if(!confirm(verb+' signed catalog '+version.slice(0,12)+'…? SNE must be stopped.'))return;
 out((action==='rollback'?'Rolling back to ':'Removing ')+version.slice(0,12)+'…','t-gold');
 fetch('/api/sne/catalog/'+action,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({version_sha256:version})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){out('Catalog '+action+' completed.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Catalog '+action+' rejected: '+e.message,'t-err')});
}

function checkSNECatalogUpdates(){
 out('Checking the authenticated SNE catalog feed…','t-gold');
 fetch('/api/sne/catalog/updates').then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(feed){
  out('Update feed  '+feed.feed_id,'t-ok');
  const available=(feed.versions||[]).filter(function(v){return v!==feed.current_version_sha256});
  if(!available.length){out('Catalog      current','t-ok');return}
  available.forEach(function(version){
   const row=document.createElement('div');row.className='t-line t-row';
   const label=document.createElement('span');label.className='t-col';label.style.flex='1';label.textContent='Available    '+version.slice(0,12)+'…';
   const install=document.createElement('span');install.className='t-action';install.textContent='[Install]';install.tabIndex=0;install.setAttribute('role','button');
   install.onclick=function(){installSNECatalogUpdate(version)};install.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();installSNECatalogUpdate(version)}};
   row.appendChild(label);row.appendChild(install);T.appendChild(row);
  });
 }).catch(function(e){out('Update check failed: '+e.message,'t-err')});
}

function installSNECatalogUpdate(version){
 if(!confirm('Install authenticated catalog '+version.slice(0,12)+'…? SNE must be stopped and the current version will remain available for rollback.'))return;
 out('Downloading and verifying signed catalog '+version.slice(0,12)+'…','t-gold');
 fetch('/api/sne/catalog/install',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({version_sha256:version})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){out('Catalog update installed; prior version retained.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Catalog update rejected: '+e.message,'t-err')});
}

function pollSNELifecycle(){
 fetch('/api/sne/lifecycle').then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(state){
  if(state.state==='ready'){out('Ready        '+state.model_id,'t-ok');setTimeout(viewSNE,300);return}
  if(state.state==='failed'){renderSNELifecycleFailure(state);return}
  out('Lifecycle    '+state.state,'t-dim');setTimeout(pollSNELifecycle,1000);
 }).catch(function(e){out('Lifecycle status unavailable: '+e.message,'t-err')});
}

function pollSNEInstall(id){
 fetch('/api/sne/install/status?id='+encodeURIComponent(id)).then(function(r){return r.json().then(function(body){
  if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})}).then(function(job){
   if(job.progress){out('Install      '+job.progress.files_done+'/'+job.progress.files_total+' files · '+fmtSize(job.progress.bytes_done)+' / '+fmtSize(job.progress.bytes_total),'t-dim')}
   if(job.state==='installed'){out('Installed    '+job.model_id,'t-ok');setTimeout(viewSNE,300);return}
   if(job.state==='failed'){
    out('Install failed: '+job.error,'t-err');
    const discard=document.createElement('div');discard.className='t-line t-action';discard.textContent='[Discard retained download]';discard.tabIndex=0;discard.setAttribute('role','button');discard.setAttribute('aria-label','Discard retained download for '+job.model_id);
    discard.onclick=function(){discardSNEPrepared(job.catalog_entry,job.model_id)};
    discard.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();discardSNEPrepared(job.catalog_entry,job.model_id)}};
    T.appendChild(discard);return
   }
   setTimeout(function(){pollSNEInstall(id)},1000);
 }).catch(function(e){out('Install status unavailable: '+e.message,'t-err')});
}

function viewScan(){
 out('Scan Results','t-gold');
 fetch('/api/findings').then(r=>r.json()).then(function(data){
  if(!data.findings||!data.findings.length){
   out('');out('No scan results. Type "scan" to run one.','t-dim');return}

  /* Count actionable items */
  let safeCount=0,safeSize=0,cautionCount=0,cautionSize=0;
  data.findings.forEach(function(f){
   if(f.severity==='safe'){safeCount++;safeSize+=f.size_bytes}
   if(f.severity==='caution'){cautionCount++;cautionSize+=f.size_bytes}
  });

  out('  '+data.findings.length+' findings · '+fmtSize(data.total_size)+' total waste','t-dim');
  out('  🟢 '+safeCount+' safe to clean ('+fmtSize(safeSize)+') · 🟡 '+cautionCount+' caution ('+fmtSize(cautionSize)+')','t-dim');
  sep();

  /* Bulk actions */
  if(safeCount>0){
   out('','t-dim');
   const bulk=document.createElement('div');bulk.className='t-line';
   const btn=document.createElement('span');btn.className='t-action';
   btn.style.cssText='color:var(--gold);font-weight:600;font-size:13px';
   btn.textContent='▸ CLEAN ALL '+safeCount+' SAFE ITEMS ('+fmtSize(safeSize)+')';
   btn.addEventListener('click',function(){cleanAllSafe(btn,data.findings)});
   bulk.appendChild(btn);T.appendChild(bulk);
   out('','t-dim');
  }
  sep();

  /* Group by category */
  const cats={};data.findings.forEach(function(f,i){f._i=i;
   if(!cats[f.category])cats[f.category]={items:[],size:0};
   cats[f.category].items.push(f);cats[f.category].size+=f.size_bytes});

  Object.keys(cats).sort(function(a,b){return cats[b].size-cats[a].size}).forEach(function(cat){
   const c=cats[cat];
   out('');out('  '+cat.toUpperCase()+' ('+c.items.length+' · '+fmtSize(c.size)+')','t-head');
   c.items.forEach(function(f){
    const row=document.createElement('div');row.className='t-line t-row';
    const sev=document.createElement('span');sev.textContent=({safe:'🟢',caution:'🟡',warning:'🟠'}[f.severity]||'⚪');
    sev.style.width='20px';
    const desc=document.createElement('span');desc.className='t-col';desc.style.flex='1';
    desc.textContent=f.description;
    if(f.advisory){desc.title=f.advisory+(f.remediation?' | Fix: '+f.remediation:'')}
    const size=document.createElement('span');size.className='t-col-r';size.textContent=f.size_human||fmtSize(f.size_bytes);
    row.appendChild(sev);row.appendChild(desc);row.appendChild(size);
    if(f.can_fix){
     const act=document.createElement('span');act.className='t-action';
     act.textContent=f.remediation==='Flag for review'?'[flag]':'['+f.remediation+']';
     act.style.marginLeft='12px';
     if(f.breaking){act.style.color='var(--warn)'}
     if(f.remediation!=='Flag for review'){act.addEventListener('click',function(){cleanIdx(act,f._i)})}
     else{act.style.cursor='default';act.style.textDecoration='none';act.style.color='var(--dim)'}
     row.appendChild(act);
    }else{
     const flag=document.createElement('span');flag.style.cssText='margin-left:12px;color:var(--dim);font-size:10px';
     flag.textContent='review';row.appendChild(flag);
    }
    T.appendChild(row);
    /* Show advisory as sub-line */
    if(f.advisory){
     const adv=document.createElement('div');adv.className='t-line';
     adv.style.cssText='padding-left:24px;font-size:10px;color:var(--dim);margin-top:-2px';
     adv.textContent=f.advisory;T.appendChild(adv)}
   });
  });
  sep();out('');
  out('  🟢 safe = always safe to delete (caches, logs, temp files)','t-dim');
  out('  🟡 caution = review first (build artifacts, old venvs)','t-dim');
  out('  🟠 warning = may affect running services (shown but not cleanable)','t-dim');
  out('');out('  Type "scan" to re-scan · "clean all" for bulk cleanup · click [clean] per item','t-dim');
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function cleanAllSafe(btn,findings){
 const safeIdx=[];
 findings.forEach(function(f,i){if(f.severity==='safe')safeIdx.push(i)});
 if(!safeIdx.length)return;
 btn.textContent='▸ CLEANING '+safeIdx.length+' ITEMS...';btn.style.color='var(--gold)';
 fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({indices:safeIdx,dry_run:false})
 }).then(r=>r.json()).then(function(d){
  btn.textContent='✓ FREED '+d.freed_human+' ('+d.cleaned+' items)';btn.style.color='var(--ok)';
  /* Reload after 2s to show updated state */
  setTimeout(function(){switchView('scan')},2000);
 }).catch(function(e){btn.textContent='✗ Error: '+e.message;btn.style.color='var(--danger)'});
}

function cleanIdx(el,idx){
 el.textContent='...';
 fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({indices:[idx],dry_run:false})
 }).then(r=>r.json()).then(function(d){
  if(d.cleaned>0){el.textContent='✓ '+d.freed_human;el.style.color='var(--ok)'}
  else{el.textContent='skip';el.style.color='var(--dim)'}
 }).catch(function(){el.textContent='err';el.style.color='var(--danger)'});
}

function viewCanonicalWorkerControl(envelope){
 const state=envelope.state||{},c=state.counters||{},b=state.board||{};
 function boundedRows(rows,limit,label){
  if(rows.length>limit)out('Showing '+limit+' of '+rows.length+' '+label+'; use sirsi router control for the complete envelope.','t-dim');
  return rows.slice(0,limit);
 }
 out('Canonical worker authority — M5','t-gold');
 const observedAge=ago(envelope.generated_at)||'time unavailable';
 out('Revision '+envelope.revision+' · observed '+(envelope.generated_at||'timestamp unavailable')+' · '+observedAge,'t-dim');
 out('State SHA-256 '+envelope.state_sha256,'t-dim');
 out('  COMPLETED / TOTAL        '+b.done_tasks+' / '+b.total_tasks+'   ('+b.pct_done+'% complete)','t-head');
 out('  IN PROGRESS / PENDING    '+c.in_progress_now+' / '+c.pending,'t-head');
 out('  BLOCKED / OPEN           '+b.blocked_tasks+' / '+b.open_items,'t-head');
 sep();
 renderCanonicalWorkerActionForm(envelope);

 out('ACTIVITY — canonical router events','t-dim');
 const events=state.activity||[];
 if(!events.length)out('  No activity events in this snapshot','t-dim');
 boundedRows(events,80,'events').forEach(function(e){
  out('  '+(e.at||'time unavailable')+' · '+(e.agent||'agent unavailable')+' · '+
      (e.task_id||'task unavailable')+' · '+(e.from||'—')+' → '+(e.to||'—')+
      (e.subject?' · '+e.subject:''),'t-out');
 });
 sep();

 out('WORKERS — live ledger classification','t-dim');
 const lanes=state.fleet||[];
 if(!lanes.length)out('  No worker lanes in this snapshot','t-dim');
 boundedRows(lanes,80,'worker lanes').forEach(function(l){
  const counts=Object.keys(l.counts||{}).sort().map(function(k){return k+': '+l.counts[k]});
  out('  '+(l.agent||'agent unavailable')+' · '+(l.activity||l.wake_state||'state unavailable')+
      ' · '+l.open_items+' open · '+(counts.join(' · ')||'no task counts')+
      (l.last_touch?' · last ledger update '+l.last_touch:''),'t-out');
 });
 sep();

 out('THREADS — canonical worker sessions','t-dim');
 const threads=state.threads||[];
 if(!threads.length)out('  No thread records in this snapshot','t-dim');
 boundedRows(threads,80,'thread records').forEach(function(thread){
  out('  '+(thread.thread_id||'thread unavailable')+' · '+(thread.agent||'agent unavailable')+
      (thread.workstream?' · '+thread.workstream:'')+' · '+(thread.stale?'stale record':'recorded')+
      (Number.isFinite(thread.idle_seconds)?' · last recorded activity '+thread.idle_seconds+'s ago':''),'t-out');
 });
 sep();

 out('TASKS — canonical router records','t-dim');
 const tasks=state.tasks||[];
 if(!tasks.length)out('  No tasks in this snapshot','t-dim');
 boundedRows(tasks,180,'tasks').forEach(function(task){
  out('  '+(task.task_id||'task unavailable')+' · '+(task.agent||'agent unavailable')+
      ' · '+(task.status||'status unavailable')+' · '+(task.phase||'phase unavailable')+
      ' · '+(task.subject||'')+' · '+(task.age||'age unavailable')+
      (task.liveness?' · '+task.liveness:''),'t-out');
 });
 sep();

 out('EVIDENCE — canonical task references','t-dim');
 const evidence=state.evidence||[];
 if(!evidence.length)out('  No evidence references in this snapshot','t-dim');
 boundedRows(evidence,180,'evidence references').forEach(function(item){
  out('  '+(item.task_id||'task unavailable')+' · '+(item.agent||'agent unavailable')+
      ' · '+(item.label||'evidence')+' · '+(item.status||'status unavailable')+
      (item.url?' · '+item.url:'')+
      (item.updated?' · updated '+item.updated:''),'t-out');
 });
 (state.data_errors||[]).forEach(function(message){out('  Source warning: '+message,'t-err')});
}

function renderCanonicalWorkerActionForm(envelope){
 const specs={
  message:[['from','From','text',true],['to','Recipient','text',true],['title','Title','text',true],['instructions','Message','textarea',false],['subject_key','Subject key','text',false],['source_item','Related item','text',false]],
  review_request:[['from','From','text',true],['to','Reviewer','text',true],['title','Review title','text',true],['instructions','Review request','textarea',false],['subject_key','Subject key','text',false],['source_item','Related item','text',false]],
  delegate:[['agent','Agent','text',true],['task_id','Task ID','text',true],['subject','Task subject','text',true]],
  claim:[['agent','Agent','text',true],['task_id','Task ID (optional; blank claims the next eligible task)','text',false],['worker','Worker identity','text',true],['thread_id','Thread ID','text',true],['ttl_seconds','Lease duration in seconds (optional)','number',false]],
  cancel_handback:[['agent','Agent','text',true],['task_id','Task ID','text',true],['lease_token','Lease token','password',true],['reason','Handback reason','textarea',true]],
  result_return:[['agent','Agent','text',true],['task_id','Task ID','text',true],['lease_token','Lease token','password',true],['result_ref','Result reference','text',true]]
 };
 const allowed=new Set((envelope.capabilities||[]).filter(function(capability){return capability.mutates===true}).map(function(capability){return capability.verb}));
 const verbs=Object.keys(specs).filter(function(verb){return allowed.has(verb)});
 if(!verbs.length)return;

 const form=document.createElement('form');form.className='worker-action-form';form.setAttribute('aria-label','Canonical worker action');
 const title=document.createElement('h3');title.className='worker-action-title';title.textContent='Act on canonical worker state';form.appendChild(title);
 const baseActionNote='M5 validates each action and returns its request-bound receipt. Actions are sent through the local Pantheon capability; the M5 credential stays server-side.';
 const note=document.createElement('p');note.className='worker-action-note';note.textContent=baseActionNote;form.appendChild(note);
 const grid=document.createElement('div');grid.className='worker-action-grid';form.appendChild(grid);
 const verbLabel=document.createElement('label');verbLabel.className='worker-action-field';verbLabel.textContent='Action';
 const verbSelect=document.createElement('select');verbSelect.name='verb';verbSelect.setAttribute('aria-label','Worker action');
 const labels={message:'Message',review_request:'Request review',delegate:'Delegate task',claim:'Claim task',cancel_handback:'Cancel / hand back',result_return:'Return result'};
 verbs.forEach(function(verb){const option=document.createElement('option');option.value=verb;option.textContent=labels[verb]||verb;verbSelect.appendChild(option)});
 verbLabel.appendChild(verbSelect);grid.appendChild(verbLabel);
 const fields=document.createElement('div');fields.className='worker-action-grid worker-action-fields';fields.style.gridColumn='1/-1';grid.appendChild(fields);
 const footer=document.createElement('div');footer.className='worker-action-footer';form.appendChild(footer);
 const submit=document.createElement('button');submit.className='worker-action-submit';submit.type='submit';submit.textContent='Send to M5';footer.appendChild(submit);
 const status=document.createElement('span');status.setAttribute('role','status');status.setAttribute('aria-live','polite');status.className='t-dim';footer.appendChild(status);
 function leaseKey(agent,taskID){return JSON.stringify([agent,taskID])}
 function activeLease(lease,expected){
  if(!lease||typeof lease.agent!=='string'||!lease.agent.trim()||typeof lease.task_id!=='string'||!lease.task_id.trim()||
     typeof lease.worker!=='string'||!lease.worker.trim()||typeof lease.thread_id!=='string'||!lease.thread_id.trim()||
     typeof lease.token!=='string'||!lease.token.trim()||!Number.isSafeInteger(lease.attempt)||lease.attempt<1)return false;
  if(typeof lease.expires!=='string')return false;
  const expiry=Date.parse(lease.expires);if(!Number.isFinite(expiry)||expiry<=Date.now())return false;
  if(expected&&((expected.agent!==undefined&&lease.agent!==expected.agent)||
     (expected.worker!==undefined&&lease.worker!==expected.worker)||
     (expected.thread_id!==undefined&&lease.thread_id!==expected.thread_id)||
     (expected.task_id&&lease.task_id!==expected.task_id)))return false;
  return true;
 }

 function drawFields(){
  while(fields.firstChild)fields.removeChild(fields.firstChild);
  note.textContent=baseActionNote;
  status.textContent='';status.className='t-dim';
  Object.keys(workerControlLeases).forEach(function(key){if(!activeLease(workerControlLeases[key]))delete workerControlLeases[key]});
  specs[verbSelect.value].forEach(function(spec){
   const name=spec[0],labelText=spec[1],kind=spec[2],required=spec[3];
   const label=document.createElement('label');label.className='worker-action-field'+(kind==='textarea'?' wide':'');label.textContent=labelText+(required?' · required':'');
   const input=document.createElement(kind==='textarea'?'textarea':'input');input.name=name;input.autocomplete=kind==='password'?'new-password':'off';
   if(kind!=='textarea')input.type=kind;
   input.required=required;
   if(kind==='number'){input.min='0';input.max='86400';input.step='1'}
   label.appendChild(input);fields.appendChild(label);
  });
  if(verbSelect.value==='cancel_handback'||verbSelect.value==='result_return'){
   const agentInput=fields.querySelector('[name=agent]'),taskInput=fields.querySelector('[name=task_id]'),tokenInput=fields.querySelector('[name=lease_token]');
   const useMatchingLease=function(){
    const key=leaseKey(agentInput.value.trim(),taskInput.value.trim());
    let lease=workerControlLeases[key];
    if(lease&&!activeLease(lease,{agent:agentInput.value.trim(),task_id:taskInput.value.trim()})){delete workerControlLeases[key];lease=null}
    if(tokenInput.dataset.controlLeaseAutofill==='true'&&!lease){tokenInput.value='';delete tokenInput.dataset.controlLeaseAutofill}
    if(lease){tokenInput.value=lease.token;tokenInput.dataset.controlLeaseAutofill='true';status.textContent='Matching active lease filled from this page session · expires '+lease.expires+'.';status.className='t-dim'}
   };
   agentInput.addEventListener('input',useMatchingLease);taskInput.addEventListener('input',useMatchingLease);
   note.textContent='M5 validates each action and returns its request-bound receipt. Enter the exact agent and task to use a matching active lease from this page session; credentials stay masked and in memory.';
  }
 }
 verbSelect.addEventListener('change',drawFields);drawFields();
 form.addEventListener('submit',function(event){
  event.preventDefault();if(submit.disabled||!form.reportValidity())return;
  if(verbSelect.value==='cancel_handback'||verbSelect.value==='result_return'){
   const agentInput=fields.querySelector('[name=agent]'),taskInput=fields.querySelector('[name=task_id]'),tokenInput=fields.querySelector('[name=lease_token]');
   if(tokenInput&&tokenInput.dataset.controlLeaseAutofill==='true'){
    const key=leaseKey(agentInput.value.trim(),taskInput.value.trim()),lease=workerControlLeases[key];
    if(!lease||!activeLease(lease,{agent:agentInput.value.trim(),task_id:taskInput.value.trim()})||tokenInput.value!==lease.token){
     delete workerControlLeases[key];tokenInput.value='';delete tokenInput.dataset.controlLeaseAutofill;
     status.textContent='The stored lease is expired or no longer matches this task. Claim again or enter a current lease.';status.className='t-err';return;
    }
   }
  }
  const payload={verb:verbSelect.value};
  Array.prototype.forEach.call(fields.querySelectorAll('[name]'),function(input){
   const value=input.value.trim();if(!value)return;
   payload[input.name]=input.type==='number'?Number(value):value;
  });
  if(Object.prototype.hasOwnProperty.call(payload,'ttl_seconds')&&!Number.isSafeInteger(payload.ttl_seconds)){status.textContent='Lease duration must be a whole number.';status.className='t-err';return}
  submit.disabled=true;submit.textContent='Sending…';submit.setAttribute('aria-busy','true');status.textContent='';status.className='t-dim';
  fetch('/api/control/action',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)})
   .then(function(response){return response.json().then(function(data){if(!response.ok){
    if(data.schema==='pantheon.worker-control-failure/v1'&&data.request_sha256&&data.receipt_sha256){const error=new Error(data.error||'M5 rejected the action');error.controlFailure=data;throw error}
    throw new Error(data.error||('HTTP '+response.status))}return data})})
   .then(function(receipt){
    if(receipt.verb!==payload.verb||!receipt.request_sha256||!receipt.receipt_sha256)throw new Error('M5 returned an incomplete or mismatched action receipt');
    let acceptedLease=null;
    if(payload.verb==='claim'){
     const lease=receipt.lease||{};
     if(!activeLease({agent:lease.agent,task_id:lease.task_id,worker:lease.worker,thread_id:lease.thread_id,
        token:lease.lease_id,expires:lease.expires,attempt:lease.attempt},{agent:payload.agent,worker:payload.worker,
        thread_id:payload.thread_id,task_id:payload.task_id||''})||receipt.task_id!==lease.task_id)
      throw new Error('M5 claim receipt omitted or mismatched its active lease proof');
     acceptedLease=lease;
    }
    status.textContent='Accepted by M5 · '+(receipt.item_id||receipt.task_id||receipt.result_ref||receipt.verb);status.className='t-ok';
    out('M5 action receipt · '+receipt.verb+' · '+(receipt.item_id||receipt.task_id||receipt.result_ref||'accepted'),'t-ok');
    out('Request SHA-256 '+receipt.request_sha256,'t-dim');out('Receipt SHA-256 '+receipt.receipt_sha256,'t-dim');
    if(payload.verb==='claim'){
     const lease=acceptedLease;
     workerControlLeases[leaseKey(lease.agent,lease.task_id)]={agent:lease.agent,task_id:lease.task_id,worker:lease.worker,
      thread_id:lease.thread_id,token:lease.lease_id,expires:lease.expires,attempt:lease.attempt};
     status.textContent='Lease held for '+lease.task_id+' through '+lease.expires+' · credential kept masked in page memory.';status.className='t-ok';
    }else if(payload.verb==='cancel_handback'||payload.verb==='result_return'){
     delete workerControlLeases[leaseKey(payload.agent,payload.task_id)];
     const tokenInput=fields.querySelector('[name=lease_token]');if(tokenInput)tokenInput.value='';
    }
    if(!footer.querySelector('.worker-action-refresh')){
     const refresh=document.createElement('button');refresh.type='button';refresh.className='worker-action-refresh';refresh.textContent='Refresh canonical state';refresh.addEventListener('click',function(){switchView('fleet')});footer.appendChild(refresh);
    }
   })
   .catch(function(error){status.textContent=error.message;status.className='t-err';
    if(error.controlFailure){const failure=error.controlFailure;out('M5 rejected the action · '+failure.error,'t-err');out('Request SHA-256 '+failure.request_sha256,'t-dim');out('Failure receipt SHA-256 '+failure.receipt_sha256,'t-dim')}
    else out('M5 action failed: '+error.message,'t-err')})
   .finally(function(){submit.disabled=false;submit.textContent='Send to M5';submit.removeAttribute('aria-busy')});
 });
 T.appendChild(form);T.scrollTop=T.scrollHeight;
}

function viewFleet(){
 out('Fleet — every lane, live','t-gold');
 fetch('/api/control').then(function(r){
  if(r.status===404)return false;
  if(!r.ok)return r.json().then(function(e){throw new Error(e.error||('HTTP '+r.status))});
  return r.json().then(function(d){
   if(!d||d.schema!=='pantheon.worker-control/v1')throw new Error('canonical control response has an unsupported schema');
   viewCanonicalWorkerControl(d);
   return true;
  });
 }).then(function(canonicalRendered){
  if(canonicalRendered)return null;
  return fetch('/api/fleet').then(function(r){
   if(!r.ok)return r.json().then(function(e){throw new Error(e.error||('HTTP '+r.status))});
   return r.json();
  });
 }).then(function(d){
  if(!d)return;
  const s=d.summary||{};
  out('');
  // Summary tiles. Percent is stated WITH its numerator and denominator so a
  // number can never be read without the count it came from.
  out('  COMPLETED / IN FLIGHT    '+s.done+' / '+s.total+'   ('+s.pct_done+'% done, '+s.in_flight+' still in flight)','t-head');
  out('  IN PROGRESS / ASSIGNED   '+s.active+' / '+(s.active+s.assigned)+'   ('+s.assigned+' assigned but not started)','t-head');
  out('  STALLED / BLOCKED        '+(s.stalled+s.blocked)+'   ('+s.stalled+' stalled · '+s.blocked+' blocked · '+s.idle_lanes+' idle lanes)','t-head');
  sep();
  out('  ACTIVITY — real status changes, in order, as they happen','t-dim');
  const acts=d.activity||[];
  if(!acts.length){
   // A seeded tracker with no events means genuine quiet; an unseeded one
   // means no baseline yet. Saying "no activity" for the second is a lie.
   out(d.seeded?'  (no status changes since this board started)':'  (baseline being taken — changes appear from the next poll)','t-dim');
  } else {
   acts.forEach(function(e){
    const row=document.createElement('div');row.className='t-line t-row';
    const at=document.createElement('span');at.className='t-col';at.style.width='90px';at.style.color='var(--dim)';at.textContent=e.at;
    const ag=document.createElement('span');ag.className='t-col';ag.style.width='170px';ag.style.color='var(--dim)';ag.textContent=e.agent;
    const sub=document.createElement('span');sub.className='t-col';sub.style.flex='1';sub.textContent=e.task_id+' — '+(e.subject||'');
    const tr=document.createElement('span');tr.className='t-col-r';
    tr.style.color=(e.to==='done')?'var(--ok)':(e.to==='blocked')?'var(--warn)':'var(--gold)';
    tr.textContent=e.from+' → '+e.to;
    row.appendChild(at);row.appendChild(ag);row.appendChild(sub);row.appendChild(tr);T.appendChild(row)});
  }
  sep();
  out('  APPLICATIONS — every lane, active first','t-dim');
  out('  '+s.lanes_working+' of '+s.lanes_total+' lanes actively working','t-dim');
  (d.lanes||[]).forEach(function(l){
   const row=document.createElement('div');row.className='t-line t-row';
   const ag=document.createElement('span');ag.className='t-col';ag.style.width='210px';ag.style.whiteSpace='nowrap';ag.textContent=l.agent;
   const st=document.createElement('span');st.className='t-col';st.style.width='200px';st.style.whiteSpace='nowrap';
   // Every state maps explicitly. A benign default turned a lane with 24 open
   // items into "stopped — no open work" the moment the vocabulary grew.
   const LBL={WORKING:'WORKING',ASSIGNED:'assigned — claimed',IDLE_WITH_WORK:'IDLE — work waiting',
              BLOCKED:'blocked',UNROUTABLE:'UNROUTABLE',COMPLETE:'complete — no open work'};
   const CLR={WORKING:'var(--ok)',ASSIGNED:'var(--ok)',IDLE_WITH_WORK:'var(--warn)',
              BLOCKED:'var(--warn)',UNROUTABLE:'var(--err)',COMPLETE:'var(--dim)'};
   st.style.color=CLR[l.state]||'var(--err)';
   st.textContent=LBL[l.state]||('unknown state: '+l.state);
   const cts=document.createElement('span');cts.className='t-col';cts.style.flex='1';cts.style.color='var(--dim)';
   let parts=[l.open+' open'];
   if(l.inbox)parts.push(l.inbox+' inbox');
   if(l.active)parts.push(l.active+' active');
   if(l.stalled)parts.push(l.stalled+' stalled');
   if(l.blocked)parts.push(l.blocked+' blocked');
   if(l.touched_ago)parts.push('touched '+l.touched_ago);
   cts.textContent=parts.join(' · ');
   row.appendChild(ag);row.appendChild(st);row.appendChild(cts);T.appendChild(row)});
 }).catch(function(e){
  const notConfigured=e.message==='fleet producer not configured';
  const card=document.createElement('section');card.className='t-empty-state';card.setAttribute('role','status');card.setAttribute('aria-live','polite');
  const title=document.createElement('div');title.className='t-empty-title';title.textContent=notConfigured?'Fleet is not connected':'Fleet is temporarily unavailable';card.appendChild(title);
  const copy=document.createElement('div');copy.className='t-empty-copy';copy.textContent=notConfigured?'The canonical fleet producer is not configured on this node yet. Pantheon is showing no worker state rather than guessing or replaying stale data.':'Pantheon could not read the canonical fleet producer. Worker state is withheld until the source responds.';card.appendChild(copy);
  if(!notConfigured){const detail=document.createElement('div');detail.className='t-empty-detail';detail.textContent='Source: '+e.message;card.appendChild(detail)}
  const actions=document.createElement('div');actions.className='t-empty-actions';
  const retry=document.createElement('button');retry.type='button';retry.className='t-empty-action';retry.textContent='Retry fleet data';retry.setAttribute('aria-label','Retry fleet data');retry.onclick=function(){switchView('fleet')};actions.appendChild(retry);
  const home=document.createElement('button');home.type='button';home.className='t-empty-action';home.textContent='Return home';home.onclick=function(){switchView('home')};actions.appendChild(home);
  card.appendChild(actions);T.appendChild(card);
 });
}

function viewGhosts(){
 out('Ghost Hunt — Scanning...','t-gold');
 fetch('/api/ghosts').then(r=>r.json()).then(function(ghosts){
  if(!ghosts.length){out('');out('No ghost remnants found. System is clean.','t-ok');return}
  let total=0;ghosts.forEach(function(g){total+=g.total_size});
  out('  '+ghosts.length+' ghosts · '+fmtSize(total)+' waste','t-dim');sep();
  ghosts.sort(function(a,b){return b.total_size-a.total_size}).forEach(function(g){
   out('');out('  👻 '+g.app_name+' — '+fmtSize(g.total_size)+' ('+g.total_files+' files)','t-head');
   g.residuals.forEach(function(r){
    const row=document.createElement('div');row.className='t-line t-row';
    const type=document.createElement('span');type.className='t-col';type.style.width='140px';type.textContent=r.type;
    const path=document.createElement('span');path.className='t-col';path.style.flex='1';path.style.color='var(--dim)';path.textContent=r.path;
    const size=document.createElement('span');size.className='t-col-r';size.textContent=fmtSize(r.size_bytes);
    row.appendChild(type);row.appendChild(path);row.appendChild(size);T.appendChild(row)});
   const cleanRow=document.createElement('div');cleanRow.className='t-line';
   const act=document.createElement('span');act.className='t-action';act.textContent='[clean all residuals]';
   act.addEventListener('click',function(){
    act.textContent='cleaning...';
    fetch('/api/ghosts/clean',{method:'POST',headers:{'Content-Type':'application/json'},
     body:JSON.stringify({app_name:g.app_name,dry_run:false})
    }).then(r=>r.json()).then(function(d){
     act.textContent='✓ freed '+d.freed_human;act.style.color='var(--ok)'
    }).catch(function(){act.textContent='error';act.style.color='var(--danger)'})});
   cleanRow.appendChild(act);T.appendChild(cleanRow)});
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function viewGuard(){
 out('Guard — System Monitor','t-gold');
 out('');out('Running diagnostics...','t-dim');
 /* Lowercase keys only — /api/doctor marshals guard.DoctorReport through its
    json tags (score/findings/check/severity/message). This block used to read
    the Go field names instead, so the score rendered "undefined/100" and the
    findings list fell through ||[] and silently discarded EVERY diagnostic:
    16 real findings shown as none, on the one screen whose whole job is to
    tell you something is wrong. Pinned by TestGuardView_ReadsDoctorJSONKeys… */
 fetch('/api/doctor').then(r=>r.json()).then(function(rpt){
  const fs=rpt.findings||[];
  const counts={0:0,1:0,2:0,3:0};fs.forEach(function(f){counts[f.severity]=(counts[f.severity]||0)+1});
  const summary=document.createElement('div');summary.className='guard-summary';summary.setAttribute('aria-label','Guard health summary');
  const score=document.createElement('div');score.className='guard-score-card';
  const scoreValue=document.createElement('div');scoreValue.className='guard-score-value';scoreValue.textContent=(rpt.score==null?'—':rpt.score)+'/100';
  const scoreLabel=document.createElement('div');scoreLabel.className='guard-score-label';scoreLabel.textContent='Health score';score.appendChild(scoreValue);score.appendChild(scoreLabel);summary.appendChild(score);
  [{key:3,label:'Critical',cls:'critical'},{key:2,label:'Warnings',cls:'warning'},{key:0,label:'Healthy',cls:'healthy'}].forEach(function(k){
   const card=document.createElement('div');card.className='guard-kpi '+k.cls;
   const value=document.createElement('div');value.className='guard-kpi-value';value.textContent=counts[k.key]||0;
   const label=document.createElement('div');label.className='guard-kpi-label';label.textContent=k.label;card.appendChild(value);card.appendChild(label);summary.appendChild(card);
  });
  T.appendChild(summary);
  function findingRow(f){
   const row=document.createElement('div');row.className='guard-finding severity-'+(f.severity==null?0:f.severity);
   const icon=document.createElement('span');icon.className='guard-finding-icon';icon.textContent=({0:'✅',1:'ℹ️',2:'⚠️',3:'🔴'}[f.severity]||'⚪');
   const name=document.createElement('span');name.className='guard-finding-name';name.textContent=f.check||'Unnamed check';
   const message=document.createElement('span');message.className='guard-finding-message';message.textContent=f.message||'No detail provided';
   row.appendChild(icon);row.appendChild(name);row.appendChild(message);return row;
  }
  const attention=fs.filter(function(f){return Number(f.severity)>=2});
  const section=document.createElement('div');section.className='guard-section-label';section.textContent=attention.length?'Needs attention':'All checks healthy';T.appendChild(section);
  const issueList=document.createElement('div');issueList.className='guard-list';
  if(attention.length){attention.forEach(function(f){issueList.appendChild(findingRow(f))})}
  else if(fs.length){fs.slice(0,3).forEach(function(f){issueList.appendChild(findingRow(f))})}
  else{const empty=document.createElement('div');empty.className='t-dim';empty.textContent='No diagnostics returned.';issueList.appendChild(empty)}
  T.appendChild(issueList);
  if(fs.length>attention.length && attention.length){
   const all=document.createElement('details');all.className='guard-all';
   const allSummary=document.createElement('summary');allSummary.textContent='Show all '+fs.length+' checks';all.appendChild(allSummary);
   const allList=document.createElement('div');allList.className='guard-list';fs.forEach(function(f){allList.appendChild(findingRow(f))});all.appendChild(allList);T.appendChild(all);
  }
  sep();out('');
  out('Process controls — type: kill node | kill electron | kill docker | kill lsp | kill build | kill ai','t-dim');
  out('Deprioritize — type: deprioritize (safe, reversible — lowers background process priority)','t-dim');
 }).catch(function(e){out('Doctor failed: '+e.message,'t-err')});
}

function viewNotifications(){
 out('Notifications','t-gold');
 fetch('/api/notifications?limit=30').then(r=>r.json()).then(function(items){
  if(!items.length){out('');out('No notifications yet.','t-dim');return}
  out('  '+items.length+' recent notifications','t-dim');sep();
  items.forEach(function(n){
   const icon=({success:'✅',error:'❌',warning:'⚠️',info:'ℹ️'}[n.severity]||'ℹ️');
   out('  '+icon+' '+n.source+' — '+n.summary+'  '+ago(n.timestamp))});
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function viewHorus(){
	out('Code Graph','t-gold');
	out('');out('Type a symbol name to search, or "graph scan" to analyze the project.','t-dim');
}

function viewVault(){
 out('Vault — Context Sandbox','t-gold');
 fetch('/api/vault/stats').then(r=>r.json()).then(function(s){
  out('  '+s.totalEntries+' entries · '+fmtSize(s.totalBytes||0)+' · '+
   Object.keys(s.tagCounts||{}).length+' tags','t-dim');
  sep();out('');out('Type a search query to find content in the vault.','t-dim');
 }).catch(function(){out('Vault not available.','t-dim')});
}

/* Ra is the control-plane name; M5 owns the canonical worker state. Keep this
   entry point useful without creating a second registry or implying that
   mutation endpoints exist before the authenticated action surface is wired. */
function viewRa(){
 out('Ra — Fleet Orchestration','t-gold');
 out('');
 out('  M5 is the single worker/router authority.','t-out');
 out('');
	out('  Pantheon reads the same canonical Fleet board; it never keeps a local worker registry.','t-dim');
 out('');
 const bridge=document.createElement('section');bridge.className='t-empty-state';bridge.setAttribute('role','status');bridge.setAttribute('aria-live','polite');
 const title=document.createElement('div');title.className='t-empty-title';title.textContent='Canonical worker board';bridge.appendChild(title);
 const copy=document.createElement('div');copy.className='t-empty-copy';copy.textContent='Inspect live worker state in Fleet. Authenticated control actions appear only when the M5 action surface is configured.';bridge.appendChild(copy);
 const actions=document.createElement('div');actions.className='t-empty-actions';
 const fleet=document.createElement('button');fleet.type='button';fleet.className='t-empty-action';fleet.textContent='Open canonical Fleet board';fleet.setAttribute('aria-label','Open canonical Fleet board');fleet.onclick=function(){switchView('fleet')};actions.appendChild(fleet);
 const home=document.createElement('button');home.type='button';home.className='t-empty-action';home.textContent='Return home';home.onclick=function(){switchView('home')};actions.appendChild(home);
 bridge.appendChild(actions);T.appendChild(bridge);
}

/* ── Command input ────────────────────────────────────── */
const input=document.getElementById('term-input');
const termSubmit=document.getElementById('term-submit');
const askCancel=document.getElementById('ask-cancel');
let askController=null;let asking=false;
let promptRouteReady=false;
function setPromptRouteReady(ready){promptRouteReady=ready;input.disabled=!ready;syncSubmitControl()}
function syncSubmitControl(){termSubmit.disabled=!input.value.trim()||asking||selectionPending||!promptRouteReady}
function submitCurrentInput(){
 if(selectionPending){out('Wait for the dashboard route change to finish before submitting.','t-dim');return}
 if(!promptRouteReady){out('Choose a configured engine route before submitting a prompt.','t-err');return}
 if(asking){out('A question is already running. Your draft remains in the input; cancel or wait before sending it.','t-dim');return}
 const raw=input.value.trim();if(!raw){syncSubmitControl();return}
 input.value='';fitInput();syncSubmitControl();exec(raw)
}
termSubmit.addEventListener('click',submitCurrentInput);
function fitInput(){
 if(input.tagName!=='TEXTAREA')return;
 input.style.height='auto';
 input.style.height=Math.min(input.scrollHeight,180)+'px';
 input.style.overflowY=input.scrollHeight>180?'auto':'hidden';
}
input.addEventListener('input',function(){syncSubmitControl();fitInput()});
fitInput();
syncSubmitControl();
askCancel.addEventListener('click',function(){if(askController)askController.abort()});

/* Typing Enter and clicking an affordance both land here. Keeping ONE dispatch
   means a clickable row can never drift from what the typed word does. */
input.addEventListener('keydown',function(e){
 if(e.key==='Escape'&&askController){e.preventDefault();askController.abort();return}
 if(e.key==='Enter'&&e.shiftKey)return;
 if(e.key!=='Enter')return;
 e.preventDefault();submitCurrentInput();
});

function exec(raw){
 /* Built-in commands */
 if(raw==='clear'){clear();return}
 if(raw==='home'){switchView('home');return}

 /* View switches */
 const viewMap={scan:'scan',ghosts:'ghosts',guard:'guard',engine:'engine',doctor:'guard',
	  notifications:'notifications',graph:'horus',horus:'horus',vault:'vault',ra:'ra',deploy:'ra'};
 if(viewMap[raw]){switchView(viewMap[raw]);return}

 /* Kill commands */
 if(raw.startsWith('kill ')){
  const target=raw.split(' ')[1];
  out('▸ kill '+target,'t-gold');
  fetch('/api/slay?target='+target+'&dry_run=false',{method:'POST'}).then(r=>r.json()).then(function(d){
   if(d.killed>0)out('✓ Killed '+d.killed+' '+target+' processes','t-ok');
   else out('No '+target+' processes found','t-dim');
  }).catch(function(e){out('✗ '+e.message,'t-err')});
  return}

 if(raw==='clean all'||raw==='clean safe'){
  out('▸ Cleaning all safe findings...','t-gold');
  fetch('/api/findings').then(r=>r.json()).then(function(data){
   const idx=[];(data.findings||[]).forEach(function(f,i){if(f.severity==='safe')idx.push(i)});
   if(!idx.length){out('No safe findings to clean.','t-dim');return}
   out('  Cleaning '+idx.length+' items...','t-dim');
   return fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({indices:idx,dry_run:false})}).then(r=>r.json()).then(function(d){
    out('✓ Freed '+d.freed_human+' ('+d.cleaned+' items cleaned)','t-ok');
    setTimeout(function(){switchView('scan')},1500)})
  }).catch(function(e){out('✗ '+e.message,'t-err')});
  return}

 if(raw==='judge'){
  out('▸ Loading findings for judgment...','t-gold');
  switchView('scan');return}

 if(raw==='renice'||raw==='renice lsp'||raw==='deprioritize'||raw==='deprioritize lsp'){
  out('▸ Deprioritize background processes (safe, reversible)','t-gold');
  fetch('/api/guard/renice?target=lsp',{method:'POST'}).then(r=>r.json()).then(function(d){
   if(d.reniced>0){out('✓ Deprioritized '+d.reniced+' background processes (safe, reversible)','t-ok');
    (d.processes||[]).forEach(function(p){out('  PID '+p.pid+' '+p.name+' — '+p.rss_human,'t-dim')})}
   else out('No background processes found to deprioritize','t-dim');
  }).catch(function(e){out('✗ '+e.message,'t-err')});
  return}

 /* Code Graph search. Keep the legacy horus alias for existing scripts. */
	if(currentView==='horus'||raw.startsWith('horus ')||raw.startsWith('graph ')){
	  const q=raw.replace(/^(?:horus|graph)\s*/,'');
  if(q==='scan'){out('▸ Scanning project...','t-gold');
   fetch('/api/horus/scan?path=.').then(r=>r.json()).then(function(g){
    const s=g.stats||g.Stats||{};
    out('  '+s.files+' files · '+s.packages+' packages · '+
     s.types+' types · '+s.functions+' functions · '+s.methods+' methods','t-dim')
   }).catch(function(e){out('Error: '+e.message,'t-err')});return}
  out('▸ search: '+q,'t-gold');
  fetch('/api/horus/query?path=.&filter='+encodeURIComponent('*'+q+'*')).then(r=>r.json()).then(function(syms){
   if(!syms||!syms.length){out('No symbols match "'+q+'"','t-dim');return}
   syms.slice(0,30).forEach(function(s){
    out('  '+s.kind+' '+(s.parent?s.parent+'.':'')+s.name+'  '+s.file+':'+s.line)})
  }).catch(function(e){out('Error: '+e.message,'t-err')});
  return}

 /* Vault search */
 if(currentView==='vault'){
  out('▸ search: '+raw,'t-gold');
  fetch('/api/vault/search?q='+encodeURIComponent(raw)+'&limit=10').then(r=>r.json()).then(function(res){
   if(!res.entries||!res.entries.length){out('No results.','t-dim');return}
   out('  '+res.totalHits+' hits','t-dim');sep();
   res.entries.forEach(function(e){
    out('  '+e.source+' ['+e.tag+']  '+e.createdAt,'t-head');
    out('  '+(e.snippet||'').substring(0,200),'t-dim');out('')})
  }).catch(function(e){out('Error: '+e.message,'t-err')});
  return}

 /* CLI command execution */
 if(running){out('A command is already running.','t-err');return}
 out('');out('▸ '+raw,'t-gold');
 const cmdMap={scan:'scan',ghosts:'ghosts',doctor:'doctor',guard:'guard',
  network:'network',hardware:'hardware',quality:'quality',dedup:'dedup'};
 const key=cmdMap[raw];
 /* Not a command — treat it as a question about this machine. The bar reads
    like a prompt, so operators type questions at it; answering "Unknown
    command" was the surface refusing an affordance it visibly offers. The
    answer is grounded in this workstation's live diagnostics and never leaves
    loopback. */
 if(!key){ask(raw);return}
 running=true;
 fetch('/api/run?cmd='+key,{method:'POST'}).then(function(r){
  if(!r.ok)return r.json().then(function(e){throw new Error(e.error)});
 }).catch(function(e){out('✗ '+e.message,'t-err');running=false});
}

/* Natural-language question about this machine, answered by the selected
   engine from this machine's live diagnostics. Every failure is stated
   plainly: a made-up answer here would be indistinguishable from a real one. */
function ask(q){
 if(asking){out('Still answering the previous question.','t-err');return}
 const requestView=currentView;
 asking=true;syncSubmitControl();
 const homeContent=currentView==='home'?document.getElementById('home-result-content'):null;
 const controller=new AbortController();askController=controller;askCancel.hidden=false;askCancel.disabled=false;
 if(homeContent){
  homeContent.replaceChildren();homeContent.setAttribute('aria-busy','true');
  const pending=document.createElement('p');pending.className='home-result-pending';pending.textContent='Submitting through the selected route…';homeContent.appendChild(pending);
 }else{out('');out('▸ '+q,'t-gold');out('  sending your question and this machine’s diagnostic context through the selected route…','t-dim')}
 fetch('/api/ask',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({question:q}),signal:controller.signal})
  .then(function(r){return r.json().then(function(d){
   if(!r.ok)throw new Error(d.error||r.statusText);return d})})
  .then(function(d){
   if(currentView!==requestView)return;
   /* Findings are rendered by the SERVER from its own diagnostic data — the
      model only chose which ones. Nothing here was written by the model, so
      no name, path or number on screen can have been paraphrased. */
   if(homeContent&&homeContent.isConnected){
    homeContent.removeAttribute('aria-busy');homeContent.replaceChildren();
    const question=document.createElement('p');question.className='home-request-echo';question.textContent='Question · '+q;homeContent.appendChild(question);
    if(d.summary){const summary=document.createElement('p');summary.className='home-result-summary';summary.textContent=d.summary;homeContent.appendChild(summary)}
    const findings=d.findings||[];
    if(findings.length){const list=document.createElement('ul');list.className='home-result-findings';findings.forEach(function(finding){const item=document.createElement('li');item.textContent=finding;list.appendChild(item)});homeContent.appendChild(list)}
    if(d.dropped){const note=document.createElement('p');note.className='home-result-note';note.textContent=d.dropped;homeContent.appendChild(note)}
    if(!d.summary&&!findings.length){const empty=document.createElement('p');empty.className='home-result-note';empty.textContent='The current diagnostics did not return a finding for this question.';homeContent.appendChild(empty)}
    if(d.receipt){
     const details=document.createElement('details');details.className='home-receipt';
     const summaryLabel=document.createElement('summary');summaryLabel.textContent='View request and route receipt';details.appendChild(summaryLabel);
     const grid=document.createElement('div');grid.className='home-receipt-grid';
     function receiptField(label,value){const key=document.createElement('span');key.className='home-receipt-key';key.textContent=label;const val=document.createElement('span');val.className='home-receipt-value';val.textContent=value||'Not returned';grid.appendChild(key);grid.appendChild(val)}
     const receipt=d.receipt;const route=receipt.route||{};const identity=receipt.identity||{};
     receiptField('Requested route',(route.requested||'unknown')+(route.requested_variant?' / '+route.requested_variant:''));
     receiptField('Executed route',(identity.engine||'unknown')+(identity.variant?' / '+identity.variant:''));
     receiptField('Fallback',route.fallback===true?'Used':route.fallback===false?'Not used':'Not reported');
     receiptField('Endpoint boundary',route.data_boundary==='on-device'?'On device':route.data_boundary==='remote'?'Remote':'Not disclosed');
     receiptField('Session',receipt.session_id);
     receiptField('Engine version',identity.engine_version);
     receiptField('Model identity',identity.model_id||d.model);
     receiptField('Identity digest',receipt.identity_digest);
     receiptField('Request SHA-256',receipt.request_sha256);
     receiptField('Completion SHA-256',receipt.completion_sha256);
     details.appendChild(grid);homeContent.appendChild(details);
    }
    return;
   }
   if(d.summary)out('  '+d.summary);
   (d.findings||[]).forEach(function(l){out('  '+l)});
   if(d.dropped)out('  ('+d.dropped+')','t-dim');
   if(!d.summary&&!(d.findings||[]).length)out('  Nothing in the current diagnostics answers that. Try "doctor".','t-dim');
   out('  — findings quoted verbatim from this machine; selection by '+d.model,'t-dim');
   if(d.receipt){
    const route=d.receipt.route||{};
    const requested=(route.requested||'unknown')+(route.requested_variant?' ('+route.requested_variant+')':'');
    const selected=(route.selected||'unknown')+(route.selected_variant?' ('+route.selected_variant+')':'');
    out('  — route '+requested+' → '+selected+(route.fallback?' · fallback':' · no fallback'),'t-dim');
   const actualBoundary=route.data_boundary==='on-device'?'loopback endpoint':route.data_boundary==='remote'?'non-loopback endpoint':'not disclosed';
   out('  — configured endpoint: '+actualBoundary,'t-dim');
   const identity=d.receipt.identity||{};
   out('  — execution '+(identity.engine||'unknown')+'/'+(identity.variant||'unknown')+' · '+(identity.engine_version||'version unknown')+' · model '+(identity.model_id||d.model||'unknown')+' · session '+(d.receipt.session_id||'unavailable'),'t-dim');
   out('  — identity digest '+(d.receipt.identity_digest||'unavailable')+' · request SHA-256 '+(d.receipt.request_sha256||'unavailable')+' · completion SHA-256 '+(d.receipt.completion_sha256||'unavailable'),'t-dim');
   }
  })
  .catch(function(e){
   if(currentView!==requestView)return;
   if(homeContent&&homeContent.isConnected){
    homeContent.removeAttribute('aria-busy');homeContent.replaceChildren();
    const failure=document.createElement('p');failure.className=e.name==='AbortError'?'home-result-note':'home-result-error';
    failure.setAttribute('role',e.name==='AbortError'?'status':'alert');
    failure.textContent=e.name==='AbortError'?'Request canceled.':e.message+' Check the selected route or choose another configured engine.';
    homeContent.appendChild(failure);
   }else if(e.name==='AbortError')out('Cancellation requested for the current prompt.','t-dim');
   else{out('✗ '+e.message,'t-err');
    out('  Other workflows: scan, ghosts, doctor, guard, network, hardware, quality, dedup, kill <target>, deprioritize','t-dim')}
  })
  .finally(function(){asking=false;if(homeContent&&homeContent.isConnected)homeContent.removeAttribute('aria-busy');if(askController===controller){askController=null;askCancel.hidden=true;askCancel.disabled=true}syncSubmitControl()});
}

/* A terminal line that runs its own command when clicked. The home screen used
   to print these as inert text, so the only way to act on a listed capability
   was to retype it by hand — the surface named eight things it could do and
   afforded none of them. */
function cmdRow(cmd,desc){
 const row=document.createElement('div');
 row.className='t-line t-cmd';
 row.tabIndex=0;row.setAttribute('role','button');
 const name=document.createElement('span');name.className='t-cmd-name';name.textContent=cmd;
 const d=document.createElement('span');d.className='t-cmd-desc';d.textContent=desc;
 row.appendChild(name);row.appendChild(d);
 const go=function(){input.value='';exec(cmd)};
 row.addEventListener('click',go);
 row.addEventListener('keydown',function(e){
  if(e.key==='Enter'||e.key===' '){e.preventDefault();go()}});
 T.appendChild(row);T.scrollTop=T.scrollHeight;
}

/* Stat tiles that map to a real destination act like the command they stand
   for. Git and Components have no view to open, so they stay plain readouts —
   deliberately, rather than half-wiring every tile. */
document.querySelectorAll('.stat-go').forEach(function(tile){
 const go=function(){exec(tile.dataset.cmd)};
 tile.addEventListener('click',go);
 tile.addEventListener('keydown',function(e){
  if(e.key==='Enter'||e.key===' '){e.preventDefault();go()}});
});

/* Land ready to type. Without this the first keystroke goes nowhere and the
   page reads as inert. */
if(currentView==='home')input.focus();

/* ── SSE ──────────────────────────────────────────────── */
if(typeof EventSource!=='undefined'){
 const es=new EventSource('/api/events');
 es.addEventListener('run_output',function(e){try{out(JSON.parse(e.data).line)}catch(x){}});
 es.addEventListener('run_complete',function(e){
  try{const d=JSON.parse(e.data);
   if(d.status==='success')out('✓ '+d.label+' ('+d.duration_ms+'ms)','t-ok');
   else out('✗ '+d.label+': '+(d.error||'failed'),'t-err');
   running=false;
   /* Auto-switch to actionable view after scan/ghost commands */
   if(d.key==='scan'){out('');out('Loading findings...','t-dim');
    setTimeout(function(){switchView('scan')},800)}
   else if(d.key==='ghosts'){setTimeout(function(){switchView('ghosts')},800)}
   else if(currentView==='scan'){setTimeout(function(){viewScan()},500)}
  }catch(x){running=false}});
}

switchView(currentView,{history:false});
})();
</script>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	pageTitle := "Home"
	if r.URL.Path == "/sne" {
		pageTitle = "SNE"
	}
	fmt.Fprint(w, pageShell(pageTitle, "home", body, s.cfg.Port))
}

// ── Page Redirects (all views are SPA now) ─────────────────────────────
// These handlers exist so direct URLs like /scan still work.
// They redirect to the SPA with the view pre-selected via JS.

func spaRedirect(w http.ResponseWriter, r *http.Request, view string) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<script>location.replace('/?view=%s')</script>`, view)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request)   { spaRedirect(w, r, "scan") }
func (s *Server) handleGhosts(w http.ResponseWriter, r *http.Request) { spaRedirect(w, r, "ghosts") }
func (s *Server) handleGuard(w http.ResponseWriter, r *http.Request)  { spaRedirect(w, r, "guard") }
func (s *Server) handleHorus(w http.ResponseWriter, r *http.Request)  { spaRedirect(w, r, "horus") }
func (s *Server) handleVault(w http.ResponseWriter, r *http.Request)  { spaRedirect(w, r, "vault") }

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	spaRedirect(w, r, "notifications")
}

// ── Legacy page code removed — all views are now rendered client-side
// in the terminal pane via the SPA entry point (handleOverview).

var _ = "legacy page handlers removed"

// Old multi-page handler bodies were here — removed in SPA rewrite.
// All rendering now happens in the terminal pane via JavaScript views.
// API endpoints in api.go, modules.go, findings.go serve the data.
// ── Helpers ─────────────────────────────────────────────────────────────

// readSteleByType reads the Stele JSONL file and returns entries matching any of the given types.
