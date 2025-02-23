# Gmail TUI

A terminal-based user interface for Gmail built with Go and the Bubble Tea framework. This application allows you to read your Gmail inbox directly from the terminal with a clean, intuitive interface.

## Images
![image](https://github.com/user-attachments/assets/b5f4b6aa-f4c3-4599-9416-2a0bd860a073)
![image](https://github.com/user-attachments/assets/e655fd02-938b-4c48-b253-fb42705ce673)


## Features

- Clean terminal user interface
- View inbox messages with subject, sender, and date
- Read full email content with scrollable viewport
- Filter emails using search
- Keyboard navigation
- OAuth2 authentication with Gmail
- Automatic token caching for persistence
- Support for plain text email content

## Prerequisites

- Go 1.19 or higher
- Gmail account
- Google Cloud Project with Gmail API enabled
- credentials.json file from Google Cloud Console

## Installation

Clone the repository:

```bash
git clone https://github.com/adityanagar10/gmail-tui.git
```

```bash
cd gmail-tui
```

Install dependencies:

```bash
go mod download
```

- Place your credentials.json file in the project root directory

- Build the application:

```bash
go build
```

- Setup Google Cloud Project

- Go to the Google Cloud Console
- Create a new project or select an existing one
- Enable the Gmail API for your project
- Create OAuth 2.0 credentials:
- Create OAuth 2.0 Client ID
- Select Desktop Application as the application type
- Download the credentials and save as credentials.json in the project directory

## Usage

Run the application:

```bash
./gmail-tui
```

## Key Bindings

- ↑/k: Move up
- ↓/j: Move down
- enter: Select/open email
- esc: Go back
- ?: Toggle help
- Q/ctrl+c: Quit
- r: Refresh emails
- pgup/pgdown: Page up/down in email view
- /: Filter emails (when in list view)

First Run
On first run, the application will:

1. Open your default browser for Gmail authentication
2. Ask you to authorize the application
3. Store the authentication token locally in token.json
4. Subsequent runs will use the cached token.

## Project Structure

The application is built using several key components:

- Bubble Tea: Terminal UI framework
- Gmail API: For fetching emails
- OAuth2: For authentication
- Lipgloss: For styling

## Security

- The application uses OAuth2 for secure authentication
- Credentials and tokens are stored locally
- Only Gmail read access is requested
- No email content is stored permanently

## Limitations

- Currently only supports plain text email content
- Limited to most recent 20 emails
- No compose or reply functionality
- Only shows the first matching text part of multipart emails

## Contributing

Feel free to submit issues, fork the repository, and create pull requests for any improvements.
