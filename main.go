package main

import (
	"bingoviewer/entle"
	"encoding/json"
	"errors"
	"fmt"
	stick "github.com/76creates/stickers"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/nokusukun/bingo"
	"github.com/sqweek/dialog"
	"os"
	"strings"
	"time"
	"unicode"
)

const RESIZE_TICK = 150

// keyMap defines a set of keybindings. To work for help it must satisfy
// key.Map. It could also very easily be a map[string]key.Binding.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Help   key.Binding
	Quit   key.Binding
	F1     key.Binding
	Escape key.Binding
	Tab    key.Binding
	Open   key.Binding
	Enter  key.Binding
	PgUp   key.Binding
	PgDn   key.Binding
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
		{k.Tab, k.Enter, k.PgUp, k.PgDn},
		{k.Up, k.Down, k.Left, k.Right}, // first column
		{k.Open, k.Help, k.Quit},        // second column
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
	F1: key.NewBinding(
		key.WithKeys("f1"),
		key.WithHelp("f1", "show messages"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "dismiss dialog"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab", "shift+tab"),
		key.WithHelp("[shift]/tab", "switch collections"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open database"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view record"),
	),
	PgUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pg up", "go up one page"),
	),
	PgDn: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pg down", "go down one page"),
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

type Message struct {
	Type      string
	Style     lipgloss.Style
	Text      string
	CreatedAt time.Time
}

func (m Message) Render() string {
	return m.Style.Render(m.Text)
}

func (m Message) FullRender() string {
	return m.Style.Render(fmt.Sprintf("%v: %v", m.CreatedAt.Format("15:04:05"), m.Text))
}

type Model struct {
	DatabaseFile     string
	driver           *bingo.Driver
	help             help.Model
	keys             keyMap
	window           screen
	state            State
	messages         []Message
	lastMsg          int
	showAllMessages  bool
	activeCollection int
	collections      []string
	columns          [][]string
	rowData          [][]any
	cleanRowData     [][]any
	table            *stick.Table

	showRecord bool
	viewport   viewport.Model
}

func NewModel() Model {
	return Model{
		help:   help.New(),
		keys:   keys,
		window: screen{},
		table:  stick.NewTable(0, 0, []string{}),
	}
}

func resizeTick() tea.Cmd {
	return tea.Tick(RESIZE_TICK*time.Millisecond, func(time.Time) tea.Msg {
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

func (m *Model) Info(msg string) {
	m.messages = append(m.messages, Message{
		Type:      "info",
		Style:     accentStyle,
		Text:      msg,
		CreatedAt: time.Now(),
	})
}

func (m *Model) Error(msg string) {
	m.messages = append(m.messages, Message{
		Type:      "error",
		Style:     errorStyle,
		Text:      msg,
		CreatedAt: time.Now(),
	})
}

func (m *Model) Success(msg string) {
	m.messages = append(m.messages, Message{
		Type:      "success",
		Style:     successStyle,
		Text:      msg,
		CreatedAt: time.Now(),
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
			m.lastMsg = len(m.messages)
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
			m.table.CursorUp()
		case key.Matches(msg, m.keys.Down):
			m.table.CursorDown()
		case key.Matches(msg, m.keys.Left):
			m.table.CursorLeft()
		case key.Matches(msg, m.keys.Right):
			m.table.CursorRight()
		case key.Matches(msg, m.keys.PgUp):
			for i := 0; i < m.window.height-8; i++ {
				m.table.CursorUp()
			}
		case key.Matches(msg, m.keys.PgDn):
			for i := 0; i < m.window.height-8; i++ {
				m.table.CursorDown()
			}
		case key.Matches(msg, m.keys.Help):
			cmd = tea.Batch(cmd, resizeTick())
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Open):
			cmd = tea.Batch(cmd, func() tea.Msg {
				return OpenDialog
			})
		case key.Matches(msg, m.keys.F1):
			m.showAllMessages = !m.showAllMessages
		case key.Matches(msg, m.keys.Escape):
			m.showRecord = false
			return m, m.ClearInfoAfter("10ms")
		case key.Matches(msg, m.keys.Tab):
			if m.collections == nil {
				break
			}
			if msg.String() == "shift+tab" {
				m.activeCollection = (m.activeCollection - 1) % len(m.collections)
				if m.activeCollection < 0 {
					m.activeCollection = len(m.collections) - 1
				}
				err := m.getData()
				if err != nil {
					m.Error(fmt.Sprintf("Failed to get columns: %v", err))
				}
				break
			}
			m.activeCollection = (m.activeCollection + 1) % len(m.collections)
			err := m.getData()
			if err != nil {
				m.Error(fmt.Sprintf("Failed to get columns: %v", err))
			}
		case key.Matches(msg, m.keys.Enter):
			if m.DatabaseFile == "" {
				break
			}
			if len(m.rowData) == 0 {
				break
			}
			m.showRecord = !m.showRecord
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
			m.Error("Open database cancelled")
		} else {
			m.Error(fmt.Sprintf("Open database failed: %v", err))
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
			m.Error(fmt.Sprintf("Open database failed: %v", err))
			driverChan <- nil
			return
		}
		driverChan <- driver
	}()

	select {
	case <-time.After(5 * time.Second):
		m.Error(fmt.Sprintf("Open database timed out, maybe it's opened sommewhere else?"))
		return m, nil
	case driver := <-driverChan:
		if driver == nil {
			return m, nil
		}
		m.driver = driver
	}

	if err != nil {
		m.Error(fmt.Sprintf("Open database failed: %v", err))
		return m, nil
	}
	colls, err := m.driver.GetCollections()
	if err != nil {
		m.Error(fmt.Sprintf("Failed to get collections: %v", err))
		return m, nil
	}
	m.collections = colls
	err = m.getData()
	if err != nil {
		m.Error(fmt.Sprintf("Failed to get columns: %v", err))
	}
	m.Success(fmt.Sprintf("Opened database: %v", load))
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
		if m.activeCollection == i {
			tabs.WriteString(activeTabStyle.Render(fmt.Sprintf("%v", coll)))
		} else {
			tabs.WriteString(tabStyle.Render(fmt.Sprintf("%v", coll)))
		}
	}
	return tabs.String()
}

type kmap map[string]any

func (kmap) Key() []byte {
	return nil
}

func (m *Model) lookup(row int) kmap {
	rowDoc := m.rowData[row]
	collection := bingo.CollectionFrom[kmap](m.driver, m.collections[m.activeCollection])
	res := collection.Query(bingo.Query[kmap]{
		Filter: func(doc kmap) bool {
			for i, col := range m.columns {
				for _, colname := range col {
					if val, ok := doc[colname]; ok {
						if fmt.Sprintf("%v", val) != rowDoc[i] {
							return false
						}
					}
				}
			}
			return true
		},
	})
	return *res.First()
}

func (m *Model) getData() error {
	cols, err := m.driver.FieldsOf(m.collections[m.activeCollection])
	if err != nil {
		return err
	}

	m.columns = cols
	loadErr := ""
	var orderedRows [][]any
	var cleanOrderedRows [][]any
	collection := bingo.CollectionFrom[kmap](m.driver, m.collections[m.activeCollection])
	collection.Query(bingo.Query[kmap]{
		Filter: func(doc kmap) bool {
			return true
		},
	}).Iter(func(docPtr *kmap) error {
		doc := *docPtr
		var row []any
		var cleanRow []any
		for _, colnames := range m.columns {
			added := false
			for _, colname := range colnames {
				if val, ok := doc[colname]; ok {
					v := strings.Map(func(r rune) rune {
						if unicode.IsPrint(r) {
							return r
						}
						return -1
					}, fmt.Sprintf("%v", val))
					row = append(row, v)
					cleanRow = append(cleanRow, val)
					added = true
					break
				}
			}
			if !added {
				row = append(row, "(None)")
				cleanRow = append(cleanRow, nil)
			}
		}
		if len(row) != len(m.columns) {
			loadErr = fmt.Sprintf("Row has %v columns, expected %v", len(row), len(m.columns))
			return nil
		}
		orderedRows = append(orderedRows, row)
		cleanOrderedRows = append(cleanOrderedRows, cleanRow)
		return nil
	})
	if loadErr != "" {
		m.Error(loadErr)
	}
	m.Info(fmt.Sprintf("Loaded %v row(s)", len(orderedRows)))
	m.rowData = orderedRows
	m.cleanRowData = cleanOrderedRows

	m.table = stick.NewTable(0, 0, m.Headers())
	m.table.SetStyles(map[stick.TableStyleKey]lipgloss.Style{
		stick.TableHeaderStyleKey: accentStyle,
		stick.TableFooterStyleKey: lipgloss.NewStyle(),
	})
	m.table, err = m.table.AddRows(m.rowData)
	if err != nil {
		m.Error(fmt.Sprintf("Failed to render table: %v", err))
	}

	return nil
}

func (m Model) Headers() []string {
	var h []string
	for _, col := range m.columns {
		h = append(h, col[0])
	}
	return h
}

func (m *Model) RenderDocumentView() string {
	if len(m.rowData) == 0 {
		return "No row data"
	}

	m.viewport.Width = m.window.width - 2
	m.viewport.Height = m.window.height - 8
	_, y := m.table.GetCursorLocation()
	doc := m.cleanRowData[y]
	var content = strings.Builder{}
	// get the widest column text width
	maxWidth := 0
	for _, colAlias := range m.columns {
		for _, colname := range colAlias {
			if len(colname) > maxWidth {
				maxWidth = len(colname)
			}
		}
	}

	for i, colAliases := range m.columns {
		//hasWritten := false
		//for _, colname := range colAliases {
		colname := colAliases[len(colAliases)-1]
		v := doc[i]
		r, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return errorStyle.Render(err.Error())
		}
		key := logoStyle.Render(colname)
		val := strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) || r == '\n' {
				return r
			}
			return -1
		}, string(r))
		if val == "null" {
			val = lipgloss.NewStyle().Foreground(lipgloss.Color("#474747")).Render(val)
		} else {
			coloredReturn := lipgloss.NewStyle().Foreground(lipgloss.Color("#e07a00")).Render("↵")
			val = strings.ReplaceAll(val, "\\n", coloredReturn+"\n")
		}
		content.WriteString(fmt.Sprintf("%v%v : %v\n", key, strings.Repeat(" ", maxWidth-len(colname)), val))
	}

	top := fmt.Sprintf("Table: %v [%v/%v]", m.collections[m.activeCollection], y+1, len(m.rowData))
	c := wordwrap.String(content.String(), m.viewport.Width-4)
	m.viewport.SetContent(fmt.Sprintf("%v\n\n%v", top, c))
	return m.viewport.View()
}

func (m *Model) RenderTable() string {
	m.table.SetWidth(m.window.width - 2)
	m.table.SetHeight(m.window.height - 8)
	if len(m.rowData) == 0 {
		return lipgloss.Place(m.window.width-2, m.window.height-10, lipgloss.Center, lipgloss.Center, "No data")
	}

	return m.table.Render()
}

var (
	barStyle         = lipgloss.NewStyle().Background(lipgloss.Color("#343434")).PaddingLeft(1).PaddingRight(1)
	accentStyle      = barStyle.Copy().Background(lipgloss.Color("#7ac0f1")).Foreground(lipgloss.Color("#141618"))
	errorStyle       = accentStyle.Copy().Background(lipgloss.Color("#ff5555")).Padding(0, 1)
	successStyle     = accentStyle.Copy().Background(lipgloss.Color("#55ff55")).Padding(0, 1)
	logoStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#7ac0f1")).Bold(true).PaddingLeft(1)
	titleBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7ac0f1"))
	tableBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#343434"))
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
	content := lipgloss.Place(center.GetWidth(), center.GetHeight(), lipgloss.Center, lipgloss.Center, "Start by opening a database with [o]")

	switch {
	case m.showAllMessages:
		var messages []string
		// reverse iterate through messages
		for i := len(m.messages) - 1; i >= 0; i-- {
			msg := m.messages[i]
			messages = append(messages, msg.FullRender())
		}
		content = lipgloss.JoinVertical(lipgloss.Top, messages...)
	case m.DatabaseFile != "":
		switch {
		case m.showRecord:
			content = lipgloss.JoinVertical(lipgloss.Top,
				m.RenderTabs(),
				tableBorderStyle.Width(m.window.width-2).Render(m.RenderDocumentView()),
			)
		default:
			content = lipgloss.JoinVertical(lipgloss.Top,
				m.RenderTabs(),
				m.RenderTable(),
			)
			content = tableBorderStyle.Render(content)
		}
	}
	center.AddRows(
		[]*stick.FlexBoxRow{
			center.NewRow().AddCells(
				[]*stick.FlexBoxCell{
					//stick.NewFlexBoxCell(1, 1).SetContent(),
					stick.NewFlexBoxCell(1, 1).SetContent(content),
				},
			),
		},
	)

	// Bottom Bar
	bottom := stick.NewFlexBox(m.window.width, 1).SetStyle(accentStyle)
	leftMsg := fmt.Sprintf("[%v:%v]", m.window.width, m.window.height)
	if m.DatabaseFile != "" {
		leftMsg = fmt.Sprintf("[%v:%v] %v row(s)", m.window.width, m.window.height, len(m.rowData))
	}
	left := accentStyle.Render(leftMsg)
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
	msg := ""
	if len(m.messages)-m.lastMsg >= 1 {
		msg = m.messages[len(m.messages)-1].Render()
		msg = fmt.Sprintf("[%v] %v", len(m.messages)-m.lastMsg, msg)
	}
	right.SetContent(accentStyle.Render(lipgloss.PlaceHorizontal(right.GetWidth()-5, lipgloss.Right, msg)))

	return lipgloss.JoinVertical(lipgloss.Top, titleBorderStyle.Render(top.Render()), center.Render(), bottom.Render(), m.help.View(m.keys))
}

func main() {
	if _, err := tea.NewProgram(NewModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Printf("Could not start program :(\n%v\n", err)
		os.Exit(1)
	}
}
