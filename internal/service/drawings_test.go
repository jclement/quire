package service

import (
	"strings"
	"testing"
)

func TestCreateAndSaveDrawing(t *testing.T) {
	svc := newTestService(t)
	d, err := svc.CreateDrawing("Auth Flow")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.Path, "attachments/") || !strings.HasSuffix(d.Path, ".excalidraw") {
		t.Errorf("path = %q", d.Path)
	}
	if d.SVGPath != d.Path+".svg" || d.Markdown != "![Auth Flow]("+d.SVGPath+")" {
		t.Errorf("drawing = %+v", d)
	}
	for _, rel := range []string{d.Path, d.SVGPath} {
		if _, err := svc.Vault.Read(rel); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
	}
	if !IsDrawingRender(d.SVGPath) || DrawingSourceFor(d.SVGPath) != d.Path {
		t.Error("render/source mapping is wrong")
	}

	scene := []byte(`{"type":"excalidraw","version":2,"elements":[{"type":"rectangle"}]}`)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)
	if _, err := svc.SaveDrawing(d.Path, scene, svg); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Vault.Read(d.SVGPath)
	if string(got.Raw) != string(svg) {
		t.Errorf("render not replaced: %q", got.Raw)
	}
}

func TestSaveDrawingRefusesBadInput(t *testing.T) {
	svc := newTestService(t)
	d, err := svc.CreateDrawing("")
	if err != nil {
		t.Fatal(err)
	}
	okScene := []byte(`{"type":"excalidraw","elements":[]}`)
	okSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	cases := []struct {
		name  string
		rel   string
		scene []byte
		svg   []byte
	}{
		{"not a drawing path", "notes/x.md", okScene, okSVG},
		{"traversal", "../x.excalidraw", okScene, okSVG},
		{"unknown drawing", "attachments/nope.excalidraw", okScene, okSVG},
		{"scene not excalidraw", d.Path, []byte(`{"type":"other"}`), okSVG},
		{"scene not json", d.Path, []byte(`nope`), okSVG},
		{"render not svg", d.Path, okScene, []byte(`<html></html>`)},
		{"render with script", d.Path, okScene, []byte(`<svg><script>alert(1)</script></svg>`)},
		{"render with handler", d.Path, okScene, []byte(`<svg onload="x()"/>`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.SaveDrawing(tc.rel, tc.scene, tc.svg); err == nil {
				t.Error("expected an error")
			}
		})
	}
	// Nothing above touched the placeholder.
	got, _ := svc.Vault.Read(d.SVGPath)
	if !strings.Contains(string(got.Raw), "Empty drawing") {
		t.Error("placeholder render was overwritten by a refused save")
	}
}
