package main

import (
	"bingoviewer/entle"
	"errors"
	"fmt"
	stick "github.com/76creates/stickers"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nokusukun/bingo"
	"github.com/sqweek/dialog"
	"os"
	"strings"
	"time"
)

// keyMap defines a set of keybindings. To work for help it must satisfy
// key.Map. It could also very easily be a map[string]key.Binding.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Help   key.Binding
	Quit   key.Binding
	Escape key.Binding
	Tab    key.Binding
	Open   key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.Help, k.Quit}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Tab}, // first column
		{k.Open, k.Help, k.Quit},               // second column
	}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "move left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "move right"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "dismiss dialog"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab", "shift+tab"),
		key.WithHelp("[shift]/tab", "switch between collections"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open database"),
	),
}

type screen struct {
	width  int
	height int
}

type State int

const (
	Normal State = iota
	ViewDocument
)

type Model struct {
	DatabaseFile string
	driver       *bingo.Driver
	help         help.Model
	keys         keyMap
	window       screen
	state        State
	info         string
	activeTab    int
	collections  []string
}

func NewModel() Model {
	return Model{
		help:   help.New(),
		keys:   keys,
		window: screen{},
	}
}

func resizeTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return tea.WindowSizeMsg{}
	})
}

func (m Model) Init() tea.Cmd {
	return resizeTick()
}

type Event int

const (
	OpenDialog Event = iota
	ClearMsg
)

func (m Model) ClearInfoAfter(s string) tea.Cmd {
	t, err := time.ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return tea.Tick(t, func(time.Time) tea.Msg {
		return ClearMsg
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case Event:
		switch msg {
		case OpenDialog:
			return m.loadDatabaseDialog()
		case ClearMsg:
			m.info = ""
		}
	case tea.WindowSizeMsg:
		if !m.help.ShowAll {
			m.help.Width = msg.Width
			m.window.width = entle.Width()
			m.window.height = entle.Height()
			cmd = tea.Batch(cmd, resizeTick())
		}
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			//m.lastKey = "↑"
		case key.Matches(msg, m.keys.Down):
			//m.lastKey = "↓"
		case key.Matches(msg, m.keys.Left):
			//m.lastKey = "←"
		case key.Matches(msg, m.keys.Right):
			//m.lastKey = "→"
		case key.Matches(msg, m.keys.Help):
			cmd = tea.Batch(cmd, resizeTick())
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Open):
			cmd = tea.Batch(cmd, func() tea.Msg {
				return OpenDialog
			})
		case key.Matches(msg, m.keys.Escape):
			m.info = ""
		case key.Matches(msg, m.keys.Tab):
			if m.collections == nil {
				break
			}
			if msg.String() == "shift+tab" {
				m.activeTab = (m.activeTab - 1) % len(m.collections)
				if m.activeTab < 0 {
					m.activeTab = len(m.collections) - 1
				}
				break
			}
			m.activeTab = (m.activeTab + 1) % len(m.collections)
		case key.Matches(msg, m.keys.Quit):
			//m.quitting = true
			return m, tea.Quit
		}
	}

	return m, cmd
}

func (m Model) loadDatabaseDialog() (tea.Model, tea.Cmd) {
	load, err := dialog.File().Filter("Bingo Database", "*.*").Title("Open Database").Load()
	if err != nil {
		if errors.Is(err, dialog.ErrCancelled) {
			m.info = errorStyle.Render("Open database cancelled")
		} else {
			m.info = errorStyle.Render(fmt.Sprintf("Open database failed: %v", err))
		}
		return m, nil
	}
	m.DatabaseFile = load

	driverChan := make(chan *bingo.Driver)
	go func() {
		driver, err := bingo.NewDriver(bingo.DriverConfiguration{
			Filename: load,
		})
		if err != nil {
			m.info = errorStyle.Render(fmt.Sprintf("Open database failed: %v", err))
			return
		}
		driverChan <- driver
	}()

	select {
	case <-time.After(1 * time.Second):
		m.info = errorStyle.Render(fmt.Sprintf("Open database timed out"))
		return m, nil
	case driver := <-driverChan:
		m.driver = driver
	}

	if err != nil {
		m.info = errorStyle.Render(fmt.Sprintf("Open database failed: %v", err))
		return m, nil
	}
	colls, err := m.driver.GetCollections()
	if err != nil {
		m.info = errorStyle.Render(fmt.Sprintf("Failed to get collections: %v", err))
		return m, nil
	}
	m.collections = colls

	m.info = succesStyle.Render(fmt.Sprintf("Opened database: %v", load))
	return m, m.ClearInfoAfter("3s")
}

func (m Model) dim() (int, int) {
	return m.window.width, m.window.height
}

var (
	activeTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#000")).Background(lipgloss.Color("#7ac0f1")).Padding(0, 1).MarginLeft(1)
	tabStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc")).Background(lipgloss.Color("#5f5f5f")).Padding(0, 1).MarginLeft(1)
)

func (m Model) RenderTabs() string {

	var tabs strings.Builder
	for i, coll := range m.collections {
		if m.activeTab == i {
			tabs.WriteString(activeTabStyle.Render(fmt.Sprintf("%v", coll)))
		} else {
			tabs.WriteString(tabStyle.Render(fmt.Sprintf("%v", coll)))
		}
	}
	return tabs.String()
}

var (
	barStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#343434")).PaddingLeft(1).PaddingRight(1)
	accentStyle = barStyle.Copy().Background(lipgloss.Color("#7ac0f1")).Foreground(lipgloss.Color("#141618"))
	errorStyle  = lipgloss.NewStyle().Background(lipgloss.Color("#ff5555")).Foreground(lipgloss.Color("#fafafa")).Padding(0, 1)
	succesStyle = lipgloss.NewStyle().Background(lipgloss.Color("#55ff55")).Foreground(lipgloss.Color("#141618")).Padding(0, 1)
	logoStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7ac0f1")).Bold(true).PaddingLeft(1)
)

func (m Model) View() string {

	// Top Bar
	top := stick.NewFlexBox(m.window.width-2, 1)
	titleBar := stick.NewFlexBoxCell(1, 1)
	bingoLogo := stick.NewFlexBoxCell(1, 1).SetContent(logoStyle.Render("Bluey!"))
	top.AddRows(
		[]*stick.FlexBoxRow{
			top.NewRow().AddCells(
				[]*stick.FlexBoxCell{
					bingoLogo,
					titleBar,
					stick.NewFlexBoxCell(1, 1).SetContent(""),
				},
			),
		},
	)
	top.ForceRecalculate()
	databaseName := m.DatabaseFile
	if databaseName == "" {
		databaseName = "No Database Opened"
	}
	titleBar = titleBar.SetContent(lipgloss.PlaceHorizontal(titleBar.GetWidth(), lipgloss.Center, databaseName+"     "))

	// Center
	center := stick.NewFlexBox(m.window.width, m.window.height-5)
	content := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Background(lipgloss.Color("#121212")).Render("woof!")
	if m.DatabaseFile != "" {
		//content = "Loading..."
		content = lipgloss.JoinVertical(lipgloss.Top,
			m.RenderTabs(),
		)
	}
	center.AddRows(
		[]*stick.FlexBoxRow{
			center.NewRow().AddCells(
				[]*stick.FlexBoxCell{
					//stick.NewFlexBoxCell(1, 1).SetContent(lipgloss.Place(center.GetWidth(), center.GetHeight(), lipgloss.Center, lipgloss.Center, content)),
					stick.NewFlexBoxCell(1, 1).SetContent(content),
				},
			),
		},
	)

	// Bottom Bar
	bottom := stick.NewFlexBox(m.window.width, 1).SetStyle(accentStyle)

	left := accentStyle.Render(fmt.Sprintf("[%v:%v]", m.window.width, m.window.height))
	right := stick.NewFlexBoxCell(1, 1)
	bottom.AddRows(
		[]*stick.FlexBoxRow{
			center.NewRow().AddCells(
				[]*stick.FlexBoxCell{
					stick.NewFlexBoxCell(1, 1).SetContent(left).SetStyle(accentStyle),
					right,
				},
			),
		},
	)
	bottom.ForceRecalculate()
	right.SetContent(accentStyle.Render(lipgloss.PlaceHorizontal(right.GetWidth()-5, lipgloss.Right, m.info)))

	borderStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7ac0f1"))
	return lipgloss.JoinVertical(lipgloss.Top, borderStyle.Render(top.Render()), center.Render(), bottom.Render(), m.help.View(m.keys))
}

func main() {
	if _, err := tea.NewProgram(NewModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Printf("Could not start program :(\n%v\n", err)
		os.Exit(1)
	}
}
