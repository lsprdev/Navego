//go:build integration

package browser

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Run with -tags=integration and an installed Chrome/Chromium. NAVEGO_TEST_CHROME
// can override its path; the browser always uses an isolated temporary profile.
func TestLegacyMenuSnapshotAndInteractions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.UserDataDir(t.TempDir()))
	if executable := os.Getenv("NAVEGO_TEST_CHROME"); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer stopAllocator()
	tab, stopTab := chromedp.NewContext(allocator)
	defer stopTab()
	html, err := os.ReadFile("testdata/legacy-menu.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(tab, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(string(html)))); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, "", 5*time.Second, 10*time.Second, 12000, 150)
	manager.browserContext = tab
	snapshot, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]Element)
	for _, element := range snapshot.Elements {
		if _, duplicate := byName[element.Name]; duplicate {
			t.Fatalf("duplicate interactive element: %s", element.Name)
		}
		byName[element.Name] = element
	}
	for _, name := range []string{"Ensino", "Botão nativo", "Excluir", "Menu por hover", "Controle por ponteiro"} {
		if byName[name].Ref == "" {
			t.Fatalf("visible interactive control %q has no ref: %+v", name, snapshot.Elements)
		}
	}
	for _, name := range []string{"Texto sem ação", "Consultar Minhas Notas", "Controle oculto", "Controle aria-hidden", "Controle desabilitado"} {
		if byName[name].Ref != "" {
			t.Fatalf("non-interactive or hidden control %q received a ref", name)
		}
	}
	if !byName["Excluir"].Sensitive || byName["Excluir"].Role != "button" {
		t.Fatal("legacy clickable controls must retain sensitive-action classification")
	}
	if password := byName["Senha"]; !password.Secret || password.Value != "" {
		t.Fatalf("password masking regressed: %+v", password)
	}
	found, err := manager.Find(ctx, "Ensino", 10)
	if err != nil {
		t.Fatal(err)
	}
	var menuRef string
	for _, match := range found.Matches {
		if match.Ref != "" {
			menuRef = match.Ref
			break
		}
	}
	if menuRef == "" {
		t.Fatal("find returned menu text without an interactive ref")
	}
	snapshot, err = manager.Hover(ctx, menuRef)
	if err != nil {
		t.Fatal(err)
	}
	var gradesRef string
	for _, element := range snapshot.Elements {
		if element.Name == "Consultar Minhas Notas" {
			gradesRef = element.Ref
		}
	}
	if gradesRef == "" {
		t.Fatal("hover did not expose a ref for the submenu")
	}
	snapshot, err = manager.Click(ctx, gradesRef)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.Text, "Consulta aberta") {
		t.Fatal("click did not activate the legacy submenu's mouseup handler")
	}
}
