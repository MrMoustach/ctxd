package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/issam/ctxd/internal/store"
)

const htmlTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>ctxd graph</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font:13px/1.4 system-ui,sans-serif;background:#0d1117;color:#e6edf3;display:flex;flex-direction:column;height:100vh;overflow:hidden}
#toolbar{display:flex;align-items:center;gap:8px;padding:6px 12px;background:#161b22;border-bottom:1px solid #30363d;flex-shrink:0;flex-wrap:wrap}
#proj-title{font-weight:600;font-size:14px;white-space:nowrap}
#stats{color:#8b949e;font-size:11px;white-space:nowrap}
#search{background:#0d1117;border:1px solid #30363d;border-radius:6px;color:#e6edf3;padding:4px 8px;font-size:12px;width:180px}
#search:focus{outline:none;border-color:#58a6ff}
#filters{display:flex;gap:4px;flex-wrap:wrap}
.fbtn{padding:2px 7px;border-radius:4px;border:1px solid #30363d;background:#161b22;color:#8b949e;font-size:11px;cursor:pointer;transition:opacity .15s}
.fbtn.on{opacity:1;border-color:currentColor}
.fbtn.off{opacity:.35}
.fbtn:hover{background:#21262d}
#main{display:flex;flex:1;min-height:0}
#canvas{flex:1;cursor:grab}
#canvas.grabbing{cursor:grabbing}
#panel{width:260px;background:#161b22;border-left:1px solid #30363d;padding:12px;overflow-y:auto;flex-shrink:0;display:none;font-size:12px}
#panel-close{float:right;cursor:pointer;color:#8b949e;font-size:18px;line-height:1;background:none;border:none;color:#8b949e}
#panel h3{font-size:13px;margin-bottom:10px;padding-right:20px;word-break:break-word}
.prow{display:flex;gap:6px;margin-bottom:5px}
.pkey{color:#8b949e;min-width:52px;flex-shrink:0}
.pval{word-break:break-all}
#nbrs h4{font-size:10px;text-transform:uppercase;letter-spacing:.06em;color:#8b949e;margin:10px 0 5px}
.nbr{padding:2px 0;cursor:pointer;color:#58a6ff}
.nbr:hover{text-decoration:underline}
.edge-type{font-size:10px;color:#8b949e;margin-left:4px}
#hud{position:absolute;bottom:12px;display:flex;gap:6px;left:50%;transform:translateX(-50%)}
.hbtn{padding:4px 10px;background:#161b22cc;border:1px solid #30363d;border-radius:6px;color:#e6edf3;font-size:12px;cursor:pointer;backdrop-filter:blur(4px)}
.hbtn:hover{background:#21262d}
#legend{position:absolute;bottom:12px;left:12px;background:#161b22cc;border:1px solid #30363d;border-radius:6px;padding:8px 10px;font-size:11px;backdrop-filter:blur(4px)}
#legend h4{font-size:10px;text-transform:uppercase;letter-spacing:.06em;color:#8b949e;margin-bottom:5px}
.lrow{display:flex;align-items:center;gap:5px;margin-bottom:3px}
.ldot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.node{cursor:pointer}
.node circle{transition:r .12s}
.node.selected circle{stroke:#fff;stroke-width:2}
.node.dim{opacity:.1}
.link{stroke:#30363d;stroke-opacity:.8}
.link.hi{stroke:#58a6ff;stroke-opacity:1}
.link.dim{opacity:.05}
.lbl{font-size:10px;fill:#8b949e;pointer-events:none;user-select:none}
</style>
</head>
<body>
<div id="toolbar">
  <span id="proj-title"></span>
  <span id="stats"></span>
  <input id="search" type="text" placeholder="Search nodes…" autocomplete="off">
  <div id="filters"></div>
</div>
<div id="main" style="position:relative">
  <svg id="canvas"></svg>
  <div id="panel">
    <button id="panel-close">×</button>
    <h3 id="pname"></h3>
    <div id="prows"></div>
    <div id="nbrs"><h4>Connected</h4><div id="nbrlist"></div></div>
  </div>
  <div id="hud">
    <button class="hbtn" id="btn-zoomin">＋</button>
    <button class="hbtn" id="btn-zoomout">－</button>
    <button class="hbtn" id="btn-fit">Fit</button>
    <button class="hbtn" id="btn-reheat">Reheat</button>
  </div>
  <div id="legend"><h4>Types</h4><div id="litems"></div></div>
</div>
<script src="https://cdn.jsdelivr.net/npm/d3@7/dist/d3.min.js"></script>
<script>
const RAW = /* CTXD_DATA */;

const COLORS = {
  file:'#3ddc97', function:'#90be6d', method:'#4cc9f0',
  route:'#ffb703', service:'#8ecae6', model:'#fb8500',
  test:'#c77dff', job:'#f4a261', command:'#e76f51', class:'#48cae4'
};
const nodeColor = t => COLORS[t] || '#e9c46a';
const nodeR = t => t === 'file' ? 7 : 5;

const nodes = RAW.nodes.map(n => ({...n}));
const byId = new Map(nodes.map(n => [n.id, n]));
const links = RAW.edges
  .map(e => ({...e, source: e.from_node_id, target: e.to_node_id}))
  .filter(e => byId.has(e.from_node_id) && byId.has(e.to_node_id));

document.getElementById('proj-title').textContent = RAW.project;
document.getElementById('stats').textContent = nodes.length + ' nodes · ' + links.length + ' edges';

// Type filters
const types = [...new Set(nodes.map(n => n.type))].sort();
const active = new Set(types);
const filtersEl = document.getElementById('filters');
types.forEach(t => {
  const b = document.createElement('button');
  b.className = 'fbtn on'; b.textContent = t; b.style.color = nodeColor(t);
  b.onclick = () => {
    active.has(t) ? active.delete(t) : active.add(t);
    b.className = 'fbtn ' + (active.has(t) ? 'on' : 'off');
    applyFilter();
  };
  filtersEl.appendChild(b);
});

// Legend
const lel = document.getElementById('litems');
types.forEach(t => {
  const r = document.createElement('div'); r.className = 'lrow';
  r.innerHTML = '<div class="ldot" style="background:' + nodeColor(t) + '"></div><span>' + t + '</span>';
  lel.appendChild(r);
});

// SVG + zoom
const svg = d3.select('#canvas');
const g = svg.append('g');
const canvasEl = document.getElementById('canvas');
let W = canvasEl.clientWidth, H = canvasEl.clientHeight;

const zoom = d3.zoom().scaleExtent([0.02, 12]).on('zoom', e => {
  g.attr('transform', e.transform);
  lbl.style('display', e.transform.k > 1.8 ? null : 'none');
});
svg.call(zoom).on('dblclick.zoom', null);
svg.on('mousedown', () => canvasEl.classList.add('grabbing'))
   .on('mouseup', () => canvasEl.classList.remove('grabbing'));

// Simulation
const sim = d3.forceSimulation(nodes)
  .force('link', d3.forceLink(links).id(n => n.id).distance(70).strength(0.4))
  .force('charge', d3.forceManyBody().strength(-220).distanceMax(500))
  .force('center', d3.forceCenter(W/2, H/2))
  .force('collide', d3.forceCollide(14));

// Edges
const link = g.append('g').selectAll('line')
  .data(links).join('line').attr('class', 'link');

// Nodes
const node = g.append('g').selectAll('g')
  .data(nodes).join('g').attr('class', 'node')
  .call(d3.drag()
    .on('start', (e,d) => { if (!e.active) sim.alphaTarget(0.3).restart(); d.fx=d.x; d.fy=d.y; })
    .on('drag',  (e,d) => { d.fx=e.x; d.fy=e.y; })
    .on('end',   (e,d) => { if (!e.active) sim.alphaTarget(0); d.fx=null; d.fy=null; }));

node.append('circle').attr('r', d => nodeR(d.type)).attr('fill', d => nodeColor(d.type));
node.append('title').text(d => '[' + d.type + '] ' + d.name + '\n' + d.file_path);

const lbl = g.append('g').selectAll('text')
  .data(nodes).join('text').attr('class', 'lbl')
  .text(d => d.name)
  .attr('dx', d => nodeR(d.type) + 3).attr('dy', '0.35em')
  .style('display', 'none');

sim.on('tick', () => {
  link.attr('x1', d => d.source.x).attr('y1', d => d.source.y)
      .attr('x2', d => d.target.x).attr('y2', d => d.target.y);
  node.attr('transform', d => 'translate(' + d.x + ',' + d.y + ')');
  lbl.attr('x', d => d.x).attr('y', d => d.y);
});

// Selection
let sel = null;
node.on('click', (e,d) => { e.stopPropagation(); select(d); });
svg.on('click', clear);

function select(d) {
  sel = d;
  const conn = new Set([d.id]);
  const connLinks = links.filter(e => {
    const s = e.source.id ?? e.source, t = e.target.id ?? e.target;
    if (s === d.id || t === d.id) { conn.add(s); conn.add(t); return true; }
    return false;
  });
  node.classed('selected', n => n === d).classed('dim', n => !conn.has(n.id));
  link.classed('hi', e => connLinks.includes(e)).classed('dim', e => !connLinks.includes(e));
  showPanel(d, conn, connLinks);
}

function clear() {
  sel = null;
  node.classed('selected dim', false);
  link.classed('hi dim', false);
  document.getElementById('panel').style.display = 'none';
}

function showPanel(d, conn, connLinks) {
  document.getElementById('panel').style.display = 'block';
  document.getElementById('pname').textContent = d.name;
  const rows = document.getElementById('prows');
  rows.innerHTML = '';
  [['type', d.type], ['file', d.file_path], ['line', d.start_line || '—']].forEach(([k,v]) => {
    rows.innerHTML += '<div class="prow"><span class="pkey">' + k + '</span><span class="pval">' + v + '</span></div>';
  });
  const nl = document.getElementById('nbrlist');
  nl.innerHTML = '';
  const edgesByNode = new Map();
  connLinks.forEach(e => {
    const s = e.source.id ?? e.source, t = e.target.id ?? e.target;
    const otherId = s === d.id ? t : s;
    if (!edgesByNode.has(otherId)) edgesByNode.set(otherId, []);
    edgesByNode.get(otherId).push(e.type);
  });
  [...conn].filter(id => id !== d.id).forEach(id => {
    const n = byId.get(id); if (!n) return;
    const types = edgesByNode.get(id) || [];
    const item = document.createElement('div'); item.className = 'nbr';
    item.innerHTML = '[' + n.type + '] ' + n.name + '<span class="edge-type">' + types.join(', ') + '</span>';
    item.onclick = () => select(n);
    nl.appendChild(item);
  });
}

document.getElementById('panel-close').onclick = clear;

// Search
document.getElementById('search').addEventListener('input', e => {
  const q = e.target.value.trim().toLowerCase();
  if (!q) { node.classed('dim', false); link.classed('dim', false); return; }
  const m = new Set(nodes.filter(n => n.name.toLowerCase().includes(q) || n.file_path.toLowerCase().includes(q)).map(n => n.id));
  node.classed('dim', n => !m.has(n.id));
  link.classed('dim', e => { const s = e.source.id??e.source, t = e.target.id??e.target; return !m.has(s)&&!m.has(t); });
});

// Type filter
function applyFilter() {
  node.style('display', n => active.has(n.type) ? null : 'none');
  link.style('display', e => {
    const sn = byId.get(e.source.id??e.source), tn = byId.get(e.target.id??e.target);
    return (sn&&active.has(sn.type))||(tn&&active.has(tn.type)) ? null : 'none';
  });
}

// HUD buttons
document.getElementById('btn-zoomin').onclick  = () => svg.transition().call(zoom.scaleBy, 1.5);
document.getElementById('btn-zoomout').onclick = () => svg.transition().call(zoom.scaleBy, 1/1.5);
document.getElementById('btn-reheat').onclick  = () => sim.alpha(0.5).restart();
document.getElementById('btn-fit').onclick = () => {
  const b = g.node().getBBox();
  if (!b.width || !b.height) return;
  const pad = 40;
  const s = Math.min((W - pad*2) / b.width, (H - pad*2) / b.height, 4);
  const tx = W/2 - s*(b.x + b.width/2), ty = H/2 - s*(b.y + b.height/2);
  svg.transition().duration(600).call(zoom.transform, d3.zoomIdentity.translate(tx,ty).scale(s));
};

window.addEventListener('resize', () => {
  W = canvasEl.clientWidth; H = canvasEl.clientHeight;
  sim.force('center', d3.forceCenter(W/2, H/2)).alpha(0.1).restart();
});
</script>
</body>
</html>`

func WriteReport(ctx context.Context, st *store.Store, project store.Project) (string, error) {
	stats, err := ProjectStats(ctx, st, project)
	if err != nil {
		return "", err
	}
	path := filepath.Join(project.RootPath, ".ctxd", "GRAPH_REPORT.md")
	var b strings.Builder
	fmt.Fprintf(&b, "# ctxd Graph Report\n\n")
	fmt.Fprintf(&b, "- Project: %s\n- Generated: %s\n- Files: %d\n- Symbols: %d\n- Edges: %d\n\n", project.Name, time.Now().UTC().Format(time.RFC3339), stats.Files, stats.Symbols, stats.Edges)
	if graphFiles := stats.NodesByType["file"]; stats.Files != graphFiles {
		fmt.Fprintf(&b, "> Warning: graph has %d file nodes but index has %d files. Run `ctxd graph build %s`.\n\n", graphFiles, stats.Files, project.Name)
	}
	sectionMap(&b, "Languages Detected", stats.Languages)
	sectionMap(&b, "Node Types", stats.NodesByType)
	sectionMap(&b, "Edge Types", stats.EdgesByType)
	sectionConnected(&b, "Top Connected Files", stats.TopFiles)
	sectionConnected(&b, "Top Connected Symbols", stats.TopSymbols)
	sectionNodes(&b, "Routes/Controllers", stats.Routes)
	sectionNodes(&b, "Services", stats.Services)
	sectionNodes(&b, "Models", stats.Models)
	sectionNodes(&b, "Jobs", stats.Jobs)
	sectionNodes(&b, "Commands", stats.Commands)
	sectionNodes(&b, "Tests", stats.Tests)
	sectionConnected(&b, "Call Graph Hotspots", stats.CallHotspots)
	sectionList(&b, "Orphan Files", stats.OrphanFiles)
	sectionConnected(&b, "High Coupling Files", stats.HighCoupling)
	return path, atomicWrite(path, []byte(b.String()))
}

func ExportJSON(ctx context.Context, st *store.Store, project store.Project) (string, error) {
	nodes, err := Nodes(ctx, st, project.ID)
	if err != nil {
		return "", err
	}
	edges, err := Edges(ctx, st, project.ID)
	if err != nil {
		return "", err
	}
	payload := map[string]any{"project": project.Name, "generated_at": time.Now().UTC().Format(time.RFC3339), "nodes": nodes, "edges": edges}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(project.RootPath, ".ctxd", "graph.json")
	return path, atomicWrite(path, append(data, '\n'))
}

func ExportHTML(ctx context.Context, st *store.Store, project store.Project) (string, error) {
	nodes, err := Nodes(ctx, st, project.ID)
	if err != nil {
		return "", err
	}
	edges, err := Edges(ctx, st, project.ID)
	if err != nil {
		return "", err
	}
	payload := map[string]any{"project": project.Name, "generated_at": time.Now().UTC().Format(time.RFC3339), "nodes": nodes, "edges": edges}
	data, _ := json.Marshal(payload)
	// Escape </script> sequences so embedded JSON can't break the script tag.
	safeData := strings.ReplaceAll(string(data), "</", `<\/`)
	html := strings.Replace(htmlTemplate, "/* CTXD_DATA */", safeData, 1)
	path := filepath.Join(project.RootPath, ".ctxd", "graph.html")
	return path, atomicWrite(path, []byte(html))
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func sectionMap(b *strings.Builder, title string, m map[string]int) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(m) == 0 {
		b.WriteString("- None\n\n")
		return
	}
	for k, v := range m {
		fmt.Fprintf(b, "- %s: %d\n", k, v)
	}
	b.WriteString("\n")
}

func sectionConnected(b *strings.Builder, title string, rows []Connected) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(rows) == 0 {
		b.WriteString("- None\n\n")
		return
	}
	for _, r := range rows {
		fmt.Fprintf(b, "- %s (%s): %d [%s]\n", r.Name, r.Type, r.Count, r.FilePath)
	}
	b.WriteString("\n")
}

func sectionNodes(b *strings.Builder, title string, rows []Node) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(rows) == 0 {
		b.WriteString("- None\n\n")
		return
	}
	for _, n := range rows {
		fmt.Fprintf(b, "- %s (%s:%d)\n", n.Name, n.FilePath, n.StartLine)
	}
	b.WriteString("\n")
}

func sectionList(b *strings.Builder, title string, rows []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(rows) == 0 {
		b.WriteString("- None\n\n")
		return
	}
	for _, r := range rows {
		fmt.Fprintf(b, "- %s\n", r)
	}
	b.WriteString("\n")
}
