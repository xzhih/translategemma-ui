package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"translategemma-ui/internal/modelstore"
)

type runtimeListItem struct {
	item   modelstore.CatalogItem
	loaded bool
}

func (i runtimeListItem) FilterValue() string {
	parts := []string{i.item.FileName, i.item.ID, i.item.Size}
	if i.item.Recommended {
		parts = append(parts, "recommended")
	}
	if i.item.Installed {
		parts = append(parts, "installed")
	}
	if i.item.Active {
		parts = append(parts, "selected")
	}
	if i.loaded {
		parts = append(parts, "loaded")
	}
	return strings.Join(parts, " ")
}

type runtimeListDelegate struct {
	titleStyle         lipgloss.Style
	titleSelectedStyle lipgloss.Style
	metaStyle          lipgloss.Style
	metaSelectedStyle  lipgloss.Style
	badgeStyle         lipgloss.Style
	selectedBadgeStyle lipgloss.Style
}

func newRuntimeCatalogList() list.Model {
	delegate := newRuntimeListDelegate()
	items := make([]list.Item, 0)
	catalogList := list.New(items, delegate, 60, 18)
	catalogList.SetShowTitle(false)
	catalogList.SetShowHelp(false)
	catalogList.SetShowFilter(false)
	catalogList.SetShowStatusBar(true)
	catalogList.SetShowPagination(true)
	catalogList.SetFilteringEnabled(false)
	catalogList.DisableQuitKeybindings()
	catalogList.SetStatusBarItemName("runtime", "runtimes")
	catalogList.Styles.StatusBar = mutedStyle.Copy().Padding(0, 1, 0, 0)
	catalogList.Styles.PaginationStyle = mutedStyle.Copy()
	catalogList.Styles.NoItems = mutedStyle.Copy()
	return catalogList
}

func newRuntimeListDelegate() runtimeListDelegate {
	return runtimeListDelegate{
		titleStyle:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
		titleSelectedStyle: lipgloss.NewStyle().Bold(true).Foreground(colorAccent),
		metaStyle:          mutedStyle.Copy(),
		metaSelectedStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		badgeStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("238")).
			Padding(0, 1),
		selectedBadgeStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(colorAccent).
			Padding(0, 1),
	}
}

func (d runtimeListDelegate) Height() int {
	return 2
}

func (d runtimeListDelegate) Spacing() int {
	return 1
}

func (d runtimeListDelegate) Update(tea.Msg, *list.Model) tea.Cmd {
	return nil
}

func (d runtimeListDelegate) ShortHelp() []key.Binding {
	return nil
}

func (d runtimeListDelegate) FullHelp() [][]key.Binding {
	return nil
}

func (d runtimeListDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(runtimeListItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	width := m.Width()
	if width <= 0 {
		return
	}

	prefix := "  "
	titleStyle := d.titleStyle
	metaStyle := d.metaStyle
	badgeStyle := d.badgeStyle
	if selected {
		prefix = "> "
		titleStyle = d.titleSelectedStyle
		metaStyle = d.metaSelectedStyle
		badgeStyle = d.selectedBadgeStyle
	}

	titleWidth := max(16, width-4)
	title := ansi.Truncate(entry.item.FileName, titleWidth, "...")
	meta := ansi.Truncate(fmt.Sprintf("%s | %s", entry.item.Size, entry.summary()), titleWidth, "...")

	titleLine := prefix + titleStyle.Render(title)
	metaLine := "  " + metaStyle.Render(meta)
	badges := entry.badges()
	if len(badges) > 0 {
		metaLine += "  " + renderBadgeRowWithStyle(badges, badgeStyle)
	}
	_, _ = fmt.Fprint(w, titleLine+"\n"+metaLine)
}

func (i runtimeListItem) summary() string {
	if i.item.Installed {
		return "ready locally"
	}
	return "download required"
}

func (i runtimeListItem) badges() []string {
	badges := make([]string, 0, 4)
	if i.item.Recommended {
		badges = append(badges, "RECOMMENDED")
	}
	if i.item.Installed {
		badges = append(badges, "LOCAL")
	} else {
		badges = append(badges, "DOWNLOAD")
	}
	if i.item.Active {
		badges = append(badges, "SELECTED")
	}
	if i.loaded {
		badges = append(badges, "LOADED")
	}
	return badges
}

func renderBadgeRowWithStyle(items []string, style lipgloss.Style) string {
	rendered := make([]string, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, style.Render(item))
	}
	return strings.Join(rendered, " ")
}

func (m *model) syncCatalogList() tea.Cmd {
	m.catalog = modelstore.Catalog(m.dataRoot, m.models, strings.TrimSpace(m.cfg.ActiveModelID), strings.TrimSpace(m.state.ActiveModelPath))

	items := make([]list.Item, 0, len(m.catalog))
	for _, item := range m.catalog {
		items = append(items, runtimeListItem{
			item:   item,
			loaded: item.Active && m.runtimeReady,
		})
	}

	cmd := m.modelList.SetItems(items)
	if len(items) == 0 {
		m.cursor = 0
		return cmd
	}

	if m.cursor < 0 || m.cursor >= len(items) {
		m.cursor = m.preferredCatalogIndex()
	}
	m.modelList.Select(m.cursor)
	return cmd
}

func (m model) preferredCatalogIndex() int {
	if idx, _, ok := matchCatalogByPath(m.catalog, m.state.ActiveModelPath); ok {
		return idx
	}
	if m.selected != nil {
		if idx, _, ok := matchCatalogByID(m.catalog, m.selected.ID); ok {
			return idx
		}
	}
	if idx, _, ok := matchCatalogByID(m.catalog, m.cfg.ActiveModelID); ok {
		return idx
	}
	if m.cursor >= 0 && m.cursor < len(m.catalog) {
		return m.cursor
	}
	if len(m.catalog) == 0 {
		return 0
	}
	return 0
}

func (m model) currentCatalogItem() (modelstore.CatalogItem, bool) {
	if len(m.catalog) == 0 {
		return modelstore.CatalogItem{}, false
	}
	idx := m.modelList.Index()
	if idx < 0 || idx >= len(m.catalog) {
		idx = clamp(m.cursor, 0, len(m.catalog)-1)
	}
	if idx < 0 || idx >= len(m.catalog) {
		return modelstore.CatalogItem{}, false
	}
	return m.catalog[idx], true
}
