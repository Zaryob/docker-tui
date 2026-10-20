package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestStructuredLogsView(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/version"):
			w.Write([]byte(`{"Version":"27.2.0","ApiVersion":"1.47"}`))
		case strings.Contains(r.URL.Path, "/containers/json"):
			w.Write([]byte(`[
				{"Id":"a1b2c3d4e5f67890","Names":["/shop_web_1"],"Image":"shop:latest","State":"running","Status":"Up 2 hours","Labels":{"com.docker.compose.project":"shop"}},
				{"Id":"b2c3d4e5f6a17890","Names":["/shop_db_1"],"Image":"postgres:16","State":"running","Status":"Up 2 hours","Labels":{"com.docker.compose.project":"shop"}},
				{"Id":"c3d4e5f6a1b27890","Names":["/cache"],"Image":"redis:alpine","State":"exited","Status":"Exited (0) 5 mins ago"}
			]`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	client, err := docker.NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}

	app := New(client, Options{})
	app.version = docker.Version{Version: "27.2.0", APIVersion: "1.47"}
	m, _ := app.Update(tea.WindowSizeMsg{Width: 110, Height: 24})
	app = m.(*App)

	t.Log("\n" + app.View())
	// 1. Verify ASCII Docker icon and 6 Logs tab in header
	v1 := ansi.Strip(app.View())
	if !strings.Contains(v1, "▪ ▪ ▪") || !strings.Contains(v1, "╰───────╯") {
		t.Errorf("header missing geometric ASCII docker icon: %q", v1[:min(100, len(v1))])
	}
	if !strings.Contains(v1, "6 Logs") || !strings.Contains(v1, "CONTAINERS") {
		t.Errorf("header missing 6 Logs or active CONTAINERS tab: %q", v1[:min(100, len(v1))])
	}

	// 2. Verify structured logs view
	l := newLogView("dtui-build", 110, 20)
	sampleLines := []string{
		"2026-10-15 19:48:39 exporting manifest list sha256:105325c03657b326383eaefe67c24f6529ce9d1eefe2162212b45...",
		"2026-10-15 19:48:39 naming to docker.io/library/dtui-built:live",
		"2026-10-15 19:48:39 unpacking to docker.io/library/dtui-built:live",
		"2026-10-15 20:20:39 build-1792084826388647000",
		"2026-10-15 20:20:42 [internal] load build definition from Dockerfile",
	}

	for i, sl := range sampleLines {
		entry := parseLogLine(sl, "dtui-build", true, time.Now(), i)
		l.entries = append(l.entries, entry)
		l.lines = append(l.lines, sl)
	}
	l.cursor = 4
	l.render()
	app.pane = l

	v2 := ansi.Strip(app.View())
	if !strings.Contains(v2, "Search logs (supports regex like \"/error|warn/\")") {
		t.Errorf("search bar placeholder missing")
	}
	if !strings.Contains(v2, "Timestamp") || !strings.Contains(v2, "Object") || !strings.Contains(v2, "Message") {
		t.Errorf("columns missing in log view: %s", v2)
	}
	if !strings.Contains(v2, "dtui-build") {
		t.Errorf("object name missing: %s", v2)
	}

	// 3. Verify regex search
	l.setQuery("/build/")
	if len(l.matches) == 0 {
		t.Errorf("expected regex matches for /build/")
	}

	// 4. Verify Tab 6
	app.pane = nil
	app.switchTab(int(tabLogs))
	if app.current().kind != tabLogs {
		t.Errorf("active tab kind = %v, want tabLogs", app.current().kind)
	}
}

func TestLogoLayout(t *testing.T) {
	for _, activeTab := range []int{0, 1, 5} {
		var tabRendered []string
		tabTitles := []string{"1 Containers", "2 Images", "3 Volumes", "4 Networks", "5 Builds", "6 Logs"}
		for i, title := range tabTitles {
			if i == activeTab {
				label := strings.ToUpper(title)
				pill := lipgloss.NewStyle().Foreground(colorActiveTabBg).Render("▌") +
					activeTabStyle.Render(" "+label+" ") +
					lipgloss.NewStyle().Foreground(colorActiveTabBg).Render("▐")
				tabRendered = append(tabRendered, pill)
			} else {
				tabRendered = append(tabRendered, tabStyle.Render(title))
			}
		}
		tabSep := " " + tabSepStyle.Render("│") + " "
		tabBar := strings.Join(tabRendered, tabSep)
		t.Logf("Active tab %d: %s", activeTab, tabBar)
	}
}
