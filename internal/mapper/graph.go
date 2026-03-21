// Package mapper generates infrastructure graph visualizations.
// Produces a self-contained HTML file with interactive network graph
// using Sigma.js (WebGL) and Graphology (MIT licensed).
//
// No external dependencies required — the HTML file includes
// all JavaScript inline and opens in any browser.
package mapper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// NodeType categorizes nodes in the infrastructure graph.
type NodeType string

const (
	NodeDevice    NodeType = "device"
	NodeApp       NodeType = "app"
	NodeGhost     NodeType = "ghost"
	NodeContainer NodeType = "container"
	NodeProcess   NodeType = "process"
	NodeCache     NodeType = "cache"
	NodeVolume    NodeType = "volume"
	NodeNetwork   NodeType = "network"
	NodeService   NodeType = "service"
)

// Node represents a single entity in the infrastructure graph.
type Node struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Type     NodeType          `json:"type"`
	Size     int64             `json:"size,omitempty"` // bytes
	Color    string            `json:"color,omitempty"`
	X        float64           `json:"x"`
	Y        float64           `json:"y"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Weight int    `json:"weight,omitempty"`
}

// InfraGraph is the complete infrastructure map.
type InfraGraph struct {
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges"`
	GeneratedAt string `json:"generated_at"`
	Hostname    string `json:"hostname"`
	Platform    string `json:"platform"`
}

// NodeColors maps node types to display colors.
var NodeColors = map[NodeType]string{
	NodeDevice:    "#C8A951", // Anubis gold
	NodeApp:       "#4A90D9", // Blue
	NodeGhost:     "#E74C3C", // Red — ghosts glow red
	NodeContainer: "#2ECC71", // Green
	NodeProcess:   "#F39C12", // Orange
	NodeCache:     "#95A5A6", // Gray
	NodeVolume:    "#8E44AD", // Purple
	NodeNetwork:   "#1ABC9C", // Teal
	NodeService:   "#3498DB", // Light blue
}

// NewGraph creates an empty infrastructure graph.
func NewGraph() *InfraGraph {
	hostname, _ := os.Hostname()
	return &InfraGraph{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Hostname:    hostname,
		Platform:    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// AddNode adds a node to the graph.
func (g *InfraGraph) AddNode(id, label string, nodeType NodeType) {
	color, ok := NodeColors[nodeType]
	if !ok {
		color = "#CCCCCC"
	}
	g.Nodes = append(g.Nodes, Node{
		ID:    id,
		Label: label,
		Type:  nodeType,
		Color: color,
	})
}

// AddEdge connects two nodes.
func (g *InfraGraph) AddEdge(source, target, label string) {
	id := fmt.Sprintf("%s->%s", source, target)
	g.Edges = append(g.Edges, Edge{
		ID:     id,
		Source: source,
		Target: target,
		Label:  label,
	})
}

// ToJSON exports the graph as JSON.
func (g *InfraGraph) ToJSON() ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}

// RenderHTML produces a self-contained HTML file with an interactive graph.
// Uses pure Canvas + JavaScript — ZERO external dependencies.
// Works from file:// protocol, no server needed.
func (g *InfraGraph) RenderHTML(outputPath string) error {
	graphJSON, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}

	colorsJSON := mustJSON(NodeColors)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>𓂀 Anubis Infrastructure Map — %s</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    background: #0D0D1A;
    color: #C8A951;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    overflow: hidden;
  }
  canvas { display: block; cursor: grab; }
  canvas:active { cursor: grabbing; }
  #header {
    position: fixed; top: 0; left: 0; right: 0; z-index: 10;
    background: linear-gradient(180deg, rgba(13,13,26,0.95) 0%%, rgba(13,13,26,0) 100%%);
    padding: 20px 30px; pointer-events: none;
  }
  #header h1 { font-size: 22px; color: #C8A951; letter-spacing: 1px; }
  #header p { font-size: 12px; color: #666; margin-top: 4px; }
  #legend {
    position: fixed; bottom: 20px; left: 20px; z-index: 10;
    background: rgba(13,13,26,0.92); border: 1px solid #333;
    border-radius: 10px; padding: 16px 20px; font-size: 12px;
    backdrop-filter: blur(8px);
  }
  .legend-item { display: flex; align-items: center; margin: 5px 0; color: #aaa; }
  .legend-dot {
    width: 10px; height: 10px; border-radius: 50%%;
    margin-right: 10px; display: inline-block;
    box-shadow: 0 0 6px currentColor;
  }
  #stats {
    position: fixed; top: 20px; right: 20px; z-index: 10;
    background: rgba(13,13,26,0.92); border: 1px solid #333;
    border-radius: 10px; padding: 16px 20px; font-size: 12px;
    text-align: right; backdrop-filter: blur(8px);
  }
  #stats .label { color: #666; }
  #stats .value { color: #C8A951; font-weight: 600; }
  #tooltip {
    position: fixed; display: none; z-index: 20;
    background: rgba(13,13,26,0.96); border: 1px solid #C8A951;
    border-radius: 8px; padding: 12px 16px; font-size: 12px;
    pointer-events: none; max-width: 300px;
    box-shadow: 0 4px 20px rgba(200,169,81,0.15);
    backdrop-filter: blur(10px);
  }
  #tooltip .name { color: #C8A951; font-weight: 600; font-size: 14px; }
  #tooltip .type { color: #888; margin-top: 2px; }
  #tooltip .meta { color: #555; margin-top: 4px; font-size: 11px; }
  #controls {
    position: fixed; bottom: 20px; right: 20px; z-index: 10;
    display: flex; gap: 8px;
  }
  #controls button {
    background: rgba(13,13,26,0.92); border: 1px solid #333;
    border-radius: 8px; padding: 8px 12px; color: #C8A951;
    cursor: pointer; font-size: 14px; backdrop-filter: blur(8px);
  }
  #controls button:hover { border-color: #C8A951; }
</style>
</head>
<body>
<div id="header">
  <h1>𓂀 Anubis Infrastructure Map</h1>
  <p>%s — %s — Generated %s</p>
</div>
<canvas id="graph"></canvas>
<div id="legend"></div>
<div id="stats"></div>
<div id="tooltip"></div>
<div id="controls">
  <button onclick="resetView()" title="Reset view">⟲</button>
  <button onclick="toggleLabels()" title="Toggle labels">Aa</button>
</div>

<script>
(function() {
  'use strict';
  const data = %s;
  const typeColors = %s;
  const canvas = document.getElementById('graph');
  const ctx = canvas.getContext('2d');
  const tooltip = document.getElementById('tooltip');
  let W, H, dpr;
  let showLabels = true;

  // --- Resize ---
  function resize() {
    dpr = window.devicePixelRatio || 1;
    W = window.innerWidth; H = window.innerHeight;
    canvas.width = W * dpr; canvas.height = H * dpr;
    canvas.style.width = W + 'px'; canvas.style.height = H + 'px';
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  window.addEventListener('resize', resize);
  resize();

  // --- Camera ---
  let camX = 0, camY = 0, camZoom = 1;
  function worldToScreen(wx, wy) {
    return [(wx - camX) * camZoom + W/2, (wy - camY) * camZoom + H/2];
  }
  function screenToWorld(sx, sy) {
    return [(sx - W/2) / camZoom + camX, (sy - H/2) / camZoom + camY];
  }

  // --- Build simulation nodes ---
  const nodes = data.nodes.map((n, i) => {
    const angle = (2 * Math.PI * i) / data.nodes.length;
    const r = 120 + Math.random() * 80;
    return {
      id: n.id, label: n.label, type: n.type,
      color: n.color || typeColors[n.type] || '#C8A951',
      size: Math.max(5, Math.min(18, Math.log2((n.size || 4096) / 1024) + 3)),
      x: Math.cos(angle) * r + (Math.random() - 0.5) * 40,
      y: Math.sin(angle) * r + (Math.random() - 0.5) * 40,
      vx: 0, vy: 0,
      metadata: n.metadata || {}
    };
  });

  const nodeMap = {};
  nodes.forEach(n => nodeMap[n.id] = n);

  const edges = data.edges.filter(e => nodeMap[e.source] && nodeMap[e.target]).map(e => ({
    source: nodeMap[e.source], target: nodeMap[e.target], label: e.label || ''
  }));

  // --- Force simulation ---
  const REPULSION = 3000;
  const SPRING_K = 0.008;
  const SPRING_LEN = 80;
  const DAMPING = 0.92;
  const CENTER_PULL = 0.001;
  let simRunning = true;
  let simSteps = 0;

  function simulate() {
    if (!simRunning) return;

    // Repulsion between all pairs (Barnes-Hut would be better for large graphs)
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i], b = nodes[j];
        let dx = b.x - a.x, dy = b.y - a.y;
        let dist = Math.sqrt(dx*dx + dy*dy) || 1;
        let force = REPULSION / (dist * dist);
        let fx = (dx / dist) * force;
        let fy = (dy / dist) * force;
        a.vx -= fx; a.vy -= fy;
        b.vx += fx; b.vy += fy;
      }
    }

    // Spring forces along edges
    edges.forEach(e => {
      let dx = e.target.x - e.source.x;
      let dy = e.target.y - e.source.y;
      let dist = Math.sqrt(dx*dx + dy*dy) || 1;
      let force = (dist - SPRING_LEN) * SPRING_K;
      let fx = (dx / dist) * force;
      let fy = (dy / dist) * force;
      e.source.vx += fx; e.source.vy += fy;
      e.target.vx -= fx; e.target.vy -= fy;
    });

    // Center pull + velocity update
    nodes.forEach(n => {
      n.vx -= n.x * CENTER_PULL;
      n.vy -= n.y * CENTER_PULL;
      n.vx *= DAMPING; n.vy *= DAMPING;
      n.x += n.vx; n.y += n.vy;
    });

    simSteps++;
    if (simSteps > 300) simRunning = false;
  }

  // --- Render ---
  let hoveredNode = null;
  let frame = 0;

  function render() {
    simulate();
    frame++;
    ctx.clearRect(0, 0, W, H);

    // Edges
    ctx.lineWidth = 0.5;
    edges.forEach(e => {
      const [x1,y1] = worldToScreen(e.source.x, e.source.y);
      const [x2,y2] = worldToScreen(e.target.x, e.target.y);
      // Fade edges based on distance from center
      const alpha = hoveredNode ? (e.source === hoveredNode || e.target === hoveredNode ? 0.6 : 0.08) : 0.2;
      ctx.strokeStyle = 'rgba(100,100,120,' + alpha + ')';
      ctx.beginPath(); ctx.moveTo(x1,y1); ctx.lineTo(x2,y2); ctx.stroke();
    });

    // Nodes
    nodes.forEach(n => {
      const [sx,sy] = worldToScreen(n.x, n.y);
      const r = n.size * camZoom;
      if (sx < -50 || sx > W+50 || sy < -50 || sy > H+50) return; // cull

      const isHovered = n === hoveredNode;
      const isConnected = hoveredNode && edges.some(e =>
        (e.source === hoveredNode && e.target === n) ||
        (e.target === hoveredNode && e.source === n));
      const dimmed = hoveredNode && !isHovered && !isConnected;

      // Glow
      if (isHovered || (!hoveredNode && r > 4)) {
        const glow = ctx.createRadialGradient(sx, sy, 0, sx, sy, r * 3);
        glow.addColorStop(0, n.color + (isHovered ? '40' : '18'));
        glow.addColorStop(1, 'transparent');
        ctx.fillStyle = glow;
        ctx.beginPath(); ctx.arc(sx, sy, r * 3, 0, Math.PI*2); ctx.fill();
      }

      // Node circle
      ctx.globalAlpha = dimmed ? 0.2 : 1;
      ctx.fillStyle = n.color;
      ctx.beginPath(); ctx.arc(sx, sy, Math.max(2, r), 0, Math.PI*2); ctx.fill();

      // Border on hover
      if (isHovered) {
        ctx.strokeStyle = '#fff';
        ctx.lineWidth = 2;
        ctx.beginPath(); ctx.arc(sx, sy, r + 2, 0, Math.PI*2); ctx.stroke();
      }

      // Labels
      if (showLabels && camZoom > 0.4 && (r > 3 || isHovered)) {
        const fontSize = Math.max(9, Math.min(13, 10 * camZoom));
        ctx.font = (isHovered ? 'bold ' : '') + fontSize + 'px -apple-system, system-ui, sans-serif';
        ctx.fillStyle = dimmed ? 'rgba(200,169,81,0.15)' : (isHovered ? '#fff' : 'rgba(200,169,81,0.7)');
        ctx.textAlign = 'center';
        ctx.fillText(n.label, sx, sy - r - 5);
      }
      ctx.globalAlpha = 1;
    });

    // Watermark
    ctx.font = '11px -apple-system, system-ui, sans-serif';
    ctx.fillStyle = 'rgba(200,169,81,0.15)';
    ctx.textAlign = 'right';
    ctx.fillText('𓂀 Sirsi Anubis • Seba Graph', W - 20, H - 12);

    requestAnimationFrame(render);
  }
  requestAnimationFrame(render);

  // --- Mouse interaction ---
  let isDragging = false, dragStartX, dragStartY, dragCamX, dragCamY;
  let dragNode = null;

  function getNodeAtMouse(mx, my) {
    const [wx, wy] = screenToWorld(mx, my);
    let closest = null, closestDist = Infinity;
    nodes.forEach(n => {
      const dx = n.x - wx, dy = n.y - wy;
      const dist = Math.sqrt(dx*dx + dy*dy);
      const hitR = (n.size + 4) / camZoom;
      if (dist < hitR && dist < closestDist) {
        closest = n; closestDist = dist;
      }
    });
    return closest;
  }

  canvas.addEventListener('mousedown', e => {
    const node = getNodeAtMouse(e.clientX, e.clientY);
    if (node) {
      dragNode = node;
      simRunning = false;
    } else {
      isDragging = true;
      dragStartX = e.clientX; dragStartY = e.clientY;
      dragCamX = camX; dragCamY = camY;
    }
  });

  canvas.addEventListener('mousemove', e => {
    if (dragNode) {
      const [wx, wy] = screenToWorld(e.clientX, e.clientY);
      dragNode.x = wx; dragNode.y = wy;
      dragNode.vx = 0; dragNode.vy = 0;
    } else if (isDragging) {
      const dx = (e.clientX - dragStartX) / camZoom;
      const dy = (e.clientY - dragStartY) / camZoom;
      camX = dragCamX - dx; camY = dragCamY - dy;
    } else {
      const node = getNodeAtMouse(e.clientX, e.clientY);
      hoveredNode = node;
      if (node) {
        tooltip.style.display = 'block';
        tooltip.style.left = (e.clientX + 15) + 'px';
        tooltip.style.top = (e.clientY + 15) + 'px';
        let html = '<div class="name">' + node.label + '</div>';
        html += '<div class="type">' + node.type + '</div>';
        if (Object.keys(node.metadata).length) {
          html += '<div class="meta">';
          for (const [k,v] of Object.entries(node.metadata)) {
            html += k + ': ' + v + '<br>';
          }
          html += '</div>';
        }
        tooltip.innerHTML = html;
        canvas.style.cursor = 'pointer';
      } else {
        tooltip.style.display = 'none';
        canvas.style.cursor = 'grab';
      }
    }
  });

  canvas.addEventListener('mouseup', () => {
    isDragging = false;
    dragNode = null;
  });

  canvas.addEventListener('wheel', e => {
    e.preventDefault();
    const factor = e.deltaY > 0 ? 0.9 : 1.1;
    camZoom = Math.max(0.1, Math.min(10, camZoom * factor));
  }, {passive: false});

  // --- Controls ---
  window.resetView = () => { camX = 0; camY = 0; camZoom = 1; };
  window.toggleLabels = () => { showLabels = !showLabels; };

  // --- Legend ---
  const typeCount = {};
  nodes.forEach(n => typeCount[n.type] = (typeCount[n.type] || 0) + 1);
  let legendHTML = '<div style="color:#C8A951;font-weight:600;margin-bottom:8px">Legend</div>';
  for (const [type, color] of Object.entries(typeColors)) {
    if (typeCount[type]) {
      legendHTML += '<div class="legend-item"><span class="legend-dot" style="background:'+color+';color:'+color+'"></span>'+type+' ('+typeCount[type]+')</div>';
    }
  }
  document.getElementById('legend').innerHTML = legendHTML;

  // --- Stats ---
  document.getElementById('stats').innerHTML =
    '<div style="color:#C8A951;font-weight:600;margin-bottom:8px">Stats</div>' +
    '<div><span class="label">Nodes: </span><span class="value">'+nodes.length+'</span></div>' +
    '<div><span class="label">Edges: </span><span class="value">'+edges.length+'</span></div>' +
    '<div><span class="label">Platform: </span><span class="value">'+data.platform+'</span></div>' +
    '<div style="margin-top:8px;color:#444;font-size:10px">Scroll to zoom • Drag to pan</div>';
})();
</script>
</body>
</html>`, g.Hostname, g.Hostname, g.Platform, g.GeneratedAt,
		string(graphJSON), colorsJSON)

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(outputPath, []byte(html), 0644)
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
