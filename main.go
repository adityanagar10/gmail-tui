package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

var (
	titleStyle = lipgloss.NewStyle().
			MarginLeft(2).
			Bold(true).
			Foreground(lipgloss.Color("#FF75B7"))

	infoStyle = lipgloss.NewStyle().
			MarginLeft(2).
			Foreground(lipgloss.Color("#9B9B9B"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			MarginTop(1).
			MarginLeft(2)
)

type Email struct {
	ID      string
	From    string
	Subject string
	Date    time.Time
	Body    string
}

func (e Email) Title() string { return e.Subject }
func (e Email) Description() string {
	return fmt.Sprintf("From: %s | %s", e.From, e.Date.Format("2006-01-02 15:04"))
}
func (e Email) FilterValue() string { return e.Subject }

type Model struct {
	list         list.Model
	help         help.Model
	keys         keyMap
	spinner      spinner.Model
	viewport     viewport.Model
	loading      bool
	selectedMail *Email
	gmailSvc     *gmail.Service
	err          error
	width        int
	height       int
}

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Select   key.Binding
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
	Fetch    key.Binding
	PageUp   key.Binding
	PageDown key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Select, k.Back, k.Fetch},
		{k.Help, k.Quit},
	}
}

func NewKeyMap() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle help")),
		Quit:     key.NewBinding(key.WithKeys("Q", "ctrl+c"), key.WithHelp("Q", "quit")),
		Fetch:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
	}
}

func initialModel(svc *gmail.Service) Model {
	keys := NewKeyMap()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("170")).
		BorderForeground(lipgloss.Color("170"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("241")).
		BorderForeground(lipgloss.Color("170"))

	l := list.New([]list.Item{}, delegate, 40, 20)
	l.SetShowTitle(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.Title = "Gmail Inbox"
	l.Styles.Title = titleStyle

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().Padding(1, 2)

	return Model{
		list:     l,
		help:     help.New(),
		keys:     keys,
		spinner:  s,
		viewport: vp,
		gmailSvc: svc,
		loading:  true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchEmails)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 6)

		if m.selectedMail != nil {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - 7
		}

	case tea.KeyMsg:
		if m.selectedMail != nil {
			switch {
			case key.Matches(msg, m.keys.Back):
				m.selectedMail = nil
			case key.Matches(msg, m.keys.PageDown):
				m.viewport.HalfViewDown()
			case key.Matches(msg, m.keys.PageUp):
				m.viewport.HalfViewUp()
			case key.Matches(msg, m.keys.Down):
				m.viewport.LineDown(1)
			case key.Matches(msg, m.keys.Up):
				m.viewport.LineUp(1)
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Fetch):
			m.loading = true
			return m, m.fetchEmails
		case key.Matches(msg, m.keys.Select):
			if i, ok := m.list.SelectedItem().(Email); ok {
				m.selectedMail = &i
				m.viewport.Width = m.width - 4
				m.viewport.Height = m.height - 7
				m.viewport.SetContent(i.Body)
			}
		}

	case EmailsMsg:
		m.loading = false
		var items []list.Item
		for _, email := range msg {
			items = append(items, email)
		}
		m.list.SetItems(items)

	case errMsg:
		m.err = msg
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if m.selectedMail == nil {
		newList, cmd := m.list.Update(msg)
		m.list = newList
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\nError: %v\n\n", m.err)
	}

	if m.loading {
		return fmt.Sprintf("\n\n   %s Loading emails...\n\n", m.spinner.View())
	}

	if m.selectedMail != nil {
		header := fmt.Sprintf(
			"%s\n%s\n%s\n%s\n",
			titleStyle.Render(m.selectedMail.Subject),
			infoStyle.Render(fmt.Sprintf("From: %s", m.selectedMail.From)),
			infoStyle.Render(fmt.Sprintf("Date: %s", m.selectedMail.Date.Format("2006-01-02 15:04"))),
			strings.Repeat("─", m.viewport.Width),
		)

		return fmt.Sprintf(
			"%s\n%s\n\n%s",
			header,
			m.viewport.View(),
			helpStyle.Render("↑/↓: scroll • esc: back • ?: help"),
		)
	}

	return fmt.Sprintf(
		"%s\n\n%s",
		m.list.View(),
		helpStyle.Render(m.help.View(m.keys)),
	)
}

type EmailsMsg []Email
type errMsg error

func getMessageBody(payload *gmail.MessagePart) string {
	if payload.Body != nil && payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	if payload.Parts != nil {
		for _, part := range payload.Parts {
			if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
				data, err := base64.URLEncoding.DecodeString(part.Body.Data)
				if err == nil {
					return string(data)
				}
			}
		}
		if len(payload.Parts) > 0 {
			return getMessageBody(payload.Parts[0])
		}
	}

	return ""
}

func (m Model) fetchEmails() tea.Msg {
	r, err := m.gmailSvc.Users.Messages.List("me").Q("").MaxResults(20).Do()
	if err != nil {
		return errMsg(err)
	}

	var emails []Email
	for _, msg := range r.Messages {
		email, err := m.gmailSvc.Users.Messages.Get("me", msg.Id).Format("full").Do()
		if err != nil {
			continue
		}

		var from, subject string
		var date time.Time

		for _, header := range email.Payload.Headers {
			switch header.Name {
			case "From":
				from = header.Value
			case "Subject":
				subject = header.Value
			case "Date":
				if d, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", header.Value); err == nil {
					date = d
				} else if d, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", header.Value); err == nil {
					date = d
				}
			}
		}

		if subject == "" {
			subject = "(no subject)"
		}

		emails = append(emails, Email{
			ID:      msg.Id,
			From:    from,
			Subject: subject,
			Date:    date,
			Body:    getMessageBody(email.Payload),
		})
	}

	return EmailsMsg(emails)
}

func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	codeChan := make(chan string)
	server := &http.Server{Addr: ":8080"}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if code := r.URL.Query().Get("code"); code != "" {
			fmt.Fprintf(w, "Authorization successful! You can close this window.")
			codeChan <- code
		}
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	config.RedirectURL = "http://localhost:8080"
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("Opening this URL in your browser: \n%v\n", authURL)

	var cmd string
	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "cmd /c start"
	case "darwin":
		cmd = "open"
	}
	exec.Command(cmd, authURL).Start()

	authCode := <-codeChan
	server.Shutdown(context.Background())

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	return tok, json.NewDecoder(f).Decode(tok)
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getGmailService() (*gmail.Service, error) {
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %v", err)
	}

	client := getClient(config)
	srv, err := gmail.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Gmail client: %v", err)
	}

	return srv, nil
}

func main() {
	log.SetOutput(os.Stderr)

	srv, err := getGmailService()
	if err != nil {
		log.Fatal(err)
	}

	p := tea.NewProgram(initialModel(srv), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
