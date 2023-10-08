package flasher

import (
	"bingoviewer/entle"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"io"
)

func FlashConfirmCommand() tea.Cmd {
	return func() tea.Msg {
		return FlashConfirmMsg{}
	}
}

type FlashEvent struct {
	Id   string
	Type any
}
type FlashMessage string
type FlashConfirmMsg struct{}
type FlashInterruptMsg struct{}

func SendFlash(id, m string) tea.Cmd {
	return func() tea.Msg {
		return FlashEvent{
			Id:   id,
			Type: FlashMessage(m),
		}
	}
}

func ConfirmFlash(id string) tea.Cmd {
	return func() tea.Msg {
		return FlashEvent{
			Id:   id,
			Type: FlashConfirmMsg{},
		}
	}
}

func interrupt(id string) tea.Cmd {
	return func() tea.Msg {
		return FlashEvent{
			Id:   id,
			Type: FlashInterruptMsg{},
		}
	}
}

type Model struct {
	Id      string
	Message string
	Active  bool
	Style   lipgloss.Style
}

func DefaultStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Padding(1, 4).
		Foreground(lipgloss.Color("#FAFAFA")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Align(lipgloss.Center)
}

type StyleOption func(lipgloss.Style) lipgloss.Style

var Error = func(style lipgloss.Style) lipgloss.Style {
	return style.BorderForeground(lipgloss.Color("#FF0000")).Blink(true)
}

var Success = func(style lipgloss.Style) lipgloss.Style {
	return style.BorderForeground(lipgloss.Color("#00FF00"))
}

func New(id string, styles ...StyleOption) Model {
	var style = DefaultStyle()
	for _, opt := range styles {
		style = opt(style)
	}

	return Model{
		Id:      id,
		Message: "",
		Active:  false,
		Style:   style,
	}
}

func (f Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FlashEvent:
		if msg.Id != f.Id {
			return f, nil
		}
		switch event := msg.Type.(type) {
		case FlashMessage:
			f.Message = string(event)
			f.Active = true
		case FlashConfirmMsg:
			f.Active = false
		case FlashInterruptMsg:
			if f.Active {
				return f, interrupt(f.Id)
			}
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc", "q":
			f.Active = false
		default:
			if !f.Active {
				return f, nil
			}
			return f, interrupt(f.Id)
		}
	}
	return f, nil
}

func (f Model) View() string {
	if !f.Active {
		return ""
	}

	block := f.Style.Render(f.Message)
	block = lipgloss.Place(entle.Width(), entle.Height(), lipgloss.Center, lipgloss.Center, block)
	//block = lipgloss.PlaceVertical(entle.Height(), lipgloss.Center, block)

	return block
}

func (f Model) RenderOn(writer io.StringWriter) {
	writer.WriteString(f.View())
}
