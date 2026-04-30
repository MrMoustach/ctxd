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
	html := `<!doctype html>
<html><head><meta charset="utf-8"><title>ctxd graph</title><style>
body{margin:0;font:14px system-ui;background:#101820;color:#eef2f3}#top{padding:12px 16px;border-bottom:1px solid #2b3a42}#graph{width:100vw;height:calc(100vh - 48px);display:block}.node{cursor:pointer}.label{font-size:11px;fill:#eef2f3;pointer-events:none}.edge{stroke:#6b7c85;stroke-opacity:.55}
</style></head><body><div id="top"></div><svg id="graph"></svg><script>
const data = ` + string(data) + `;
document.getElementById('top').textContent = data.project + ' graph: ' + data.nodes.length + ' nodes, ' + data.edges.length + ' edges';
const svg=document.getElementById('graph'), W=innerWidth, H=innerHeight-48; svg.setAttribute('viewBox','0 0 '+W+' '+H);
const nodes=data.nodes.map((n,i)=>({...n,x:W/2+Math.cos(i)*W/4,y:H/2+Math.sin(i)*H/4,vx:0,vy:0}));
const by=new Map(nodes.map(n=>[n.id,n])); const edges=data.edges.map(e=>({...e,source:by.get(e.from_node_id),target:by.get(e.to_node_id)})).filter(e=>e.source&&e.target);
function color(t){return {file:'#3ddc97',route:'#ffb703',service:'#8ecae6',model:'#fb8500',test:'#c77dff',function:'#90be6d',method:'#4cc9f0'}[t]||'#e9c46a'}
for(let k=0;k<240;k++){for(const e of edges){let dx=e.target.x-e.source.x,dy=e.target.y-e.source.y,d=Math.hypot(dx,dy)||1,f=(d-120)*.002;e.source.vx+=dx/d*f;e.source.vy+=dy/d*f;e.target.vx-=dx/d*f;e.target.vy-=dy/d*f}for(const a of nodes){for(const b of nodes){if(a===b)continue;let dx=a.x-b.x,dy=a.y-b.y,d=Math.max(20,Math.hypot(dx,dy)),f=45/(d*d);a.vx+=dx/d*f;a.vy+=dy/d*f}a.vx+=(W/2-a.x)*.0008;a.vy+=(H/2-a.y)*.0008;a.x+=a.vx;a.y+=a.vy;a.vx*=.85;a.vy*=.85}}
for(const e of edges){let l=document.createElementNS('http://www.w3.org/2000/svg','line');l.setAttribute('class','edge');l.setAttribute('x1',e.source.x);l.setAttribute('y1',e.source.y);l.setAttribute('x2',e.target.x);l.setAttribute('y2',e.target.y);svg.appendChild(l)}
for(const n of nodes){let c=document.createElementNS('http://www.w3.org/2000/svg','circle');c.setAttribute('class','node');c.setAttribute('cx',n.x);c.setAttribute('cy',n.y);c.setAttribute('r',n.type==='file'?5:4);c.setAttribute('fill',color(n.type));c.innerHTML='<title>'+n.type+' '+n.name+' '+n.file_path+'</title>';svg.appendChild(c);let t=document.createElementNS('http://www.w3.org/2000/svg','text');t.setAttribute('class','label');t.setAttribute('x',n.x+6);t.setAttribute('y',n.y+4);t.textContent=n.name;svg.appendChild(t)}
</script></body></html>`
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
