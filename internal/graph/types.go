package graph

type Node struct {
	ID            int64  `json:"id"`
	ProjectID     int64  `json:"project_id"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name,omitempty"`
	FilePath      string `json:"file_path"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	MetadataJSON  string `json:"metadata_json,omitempty"`
}

type Edge struct {
	ID           int64   `json:"id"`
	ProjectID    int64   `json:"project_id"`
	FromNodeID   int64   `json:"from_node_id"`
	ToNodeID     int64   `json:"to_node_id"`
	Type         string  `json:"type"`
	Confidence   float64 `json:"confidence"`
	MetadataJSON string  `json:"metadata_json,omitempty"`
}

type ParsedFile struct {
	Path    string
	Lang    string
	Content string
	Nodes   []Node
	Imports []string
	Calls   []Call
	Routes  []Route
	Uses    []Use
}

type Call struct {
	FromName string
	ToName   string
	Line     int
	Raw      string
}

type Route struct {
	Method     string
	URI        string
	Controller string
	Action     string
	Line       int
	Raw        string
}

type Use struct {
	FromName string
	ToName   string
	Type     string
	Line     int
	Raw      string
}

type Stats struct {
	Project      string         `json:"project"`
	Files        int            `json:"files"`
	Symbols      int            `json:"symbols"`
	Edges        int            `json:"edges"`
	Languages    map[string]int `json:"languages"`
	NodesByType  map[string]int `json:"nodes_by_type"`
	EdgesByType  map[string]int `json:"edges_by_type"`
	HasGraphData bool           `json:"has_graph_data"`
	TopFiles     []Connected    `json:"top_files,omitempty"`
	TopSymbols   []Connected    `json:"top_symbols,omitempty"`
	OrphanFiles  []string       `json:"orphan_files,omitempty"`
	HighCoupling []Connected    `json:"high_coupling_files,omitempty"`
	Routes       []Node         `json:"routes,omitempty"`
	Services     []Node         `json:"services,omitempty"`
	Models       []Node         `json:"models,omitempty"`
	Jobs         []Node         `json:"jobs,omitempty"`
	Commands     []Node         `json:"commands,omitempty"`
	Tests        []Node         `json:"tests,omitempty"`
	CallHotspots []Connected    `json:"call_graph_hotspots,omitempty"`
}

type Connected struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	Count    int    `json:"count"`
}
