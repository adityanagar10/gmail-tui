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

	selectedStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#FF75B7")).
			Padding(0, 0, 0, 1)

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
}

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Back   key.Binding
	Quit   key.Binding
	Help   key.Binding
	Fetch  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Select},
		{k.Back, k.Fetch},
		{k.Help, k.Quit},
	}
}

func NewKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Fetch: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

func initialModel(svc *gmail.Service) Model {
	keys := NewKeyMap()
	help := help.New()
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

	return Model{
		list:     l,
		help:     help,
		keys:     keys,
		spinner:  s,
		gmailSvc: svc,
		loading:  true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchEmails,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.selectedMail != nil {
			switch {
			case key.Matches(msg, m.keys.Back):
				m.selectedMail = nil
				return m, nil
			}
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
				return m, nil
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
		return fmt.Sprintf(
			"\n%s\n\n%s\n\n%s\n\n%s\n\nPress ESC to go back\n",
			titleStyle.Render(m.selectedMail.Subject),
			infoStyle.Render(fmt.Sprintf("From: %s", m.selectedMail.From)),
			infoStyle.Render(fmt.Sprintf("Date: %s", m.selectedMail.Date.Format("2006-01-02 15:04"))),
			m.selectedMail.Body,
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
		if err != nil {
			return ""
		}
		return string(data)
	}

	if payload.Parts != nil {
		for _, part := range payload.Parts {
			if part.MimeType == "text/plain" {
				if part.Body != nil && part.Body.Data != "" {
					data, err := base64.URLEncoding.DecodeString(part.Body.Data)
					if err != nil {
						continue
					}
					return string(data)
				}
			}
		}
		// If no text/plain, try first part
		if len(payload.Parts) > 0 {
			return getMessageBody(payload.Parts[0])
		}
	}

	return ""
}

func (m Model) fetchEmails() tea.Msg {
	user := "me"
	r, err := m.gmailSvc.Users.Messages.List(user).Q("").MaxResults(20).Do()
	if err != nil {
		log.Printf("Error listing messages: %v", err)
		return errMsg(err)
	}

	var emails []Email
	for _, msg := range r.Messages {
		email, err := m.gmailSvc.Users.Messages.Get(user, msg.Id).Format("full").Do()
		if err != nil {
			log.Printf("Error getting message %s: %v", msg.Id, err)
			continue
		}

		var from, subject string
		var date time.Time

		// Extract headers
		for _, header := range email.Payload.Headers {
			switch header.Name {
			case "From":
				from = header.Value
			case "Subject":
				subject = header.Value
			case "Date":
				parsedDate, err := time.Parse("Mon, 2 Jan 2006 15:04:05 -0700", header.Value)
				if err == nil {
					date = parsedDate
				} else {
					// Try alternate date format
					parsedDate, err = time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", header.Value)
					if err == nil {
						date = parsedDate
					}
				}
			}
		}

		// Extract body
		body := getMessageBody(email.Payload)

		if subject == "" {
			subject = "(no subject)"
		}

		emails = append(emails, Email{
			ID:      msg.Id,
			From:    from,
			Subject: subject,
			Date:    date,
			Body:    body,
		})
	}

	log.Printf("Fetched %d emails", len(emails))
	for i, email := range emails {
		log.Printf("Email %d: Subject: %s, From: %s", i+1, email.Subject, email.From)
	}
	return EmailsMsg(emails)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	// Create channel to receive the auth code
	codeChan := make(chan string)

	// Start local server to handle the redirect
	server := &http.Server{Addr: ":8080"}

	// Handle the OAuth2 callback
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code != "" {
			fmt.Fprintf(w, "Authorization successful! You can close this window.")
			codeChan <- code
		}
	})

	// Start server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Set the correct redirect URL
	config.RedirectURL = "http://localhost:8080"
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("Opening this URL in your browser: \n%v\n", authURL)

	// Try to open the URL in the default browser
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

	// Wait for the code
	authCode := <-codeChan

	// Shutdown the server
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
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
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
