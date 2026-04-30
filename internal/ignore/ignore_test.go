package ignore

import "testing"

func TestDefaultIgnores(t *testing.T) {
	m := New(t.TempDir())
	cases := []string{".git/config", "node_modules/pkg/a.js", "vendor/lib/a.php", ".env", "app.js.map", "image.png", "package-lock.json"}
	for _, c := range cases {
		if !m.Ignored(c, false) {
			t.Fatalf("expected %s to be ignored", c)
		}
	}
}
