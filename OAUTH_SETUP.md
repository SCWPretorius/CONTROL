# Google OAuth Automatic Setup

## Overview

The assistant now supports interactive OAuth flow on first startup. When Google Workspace tools are configured but `GOOGLE_OAUTH_ACCESS_TOKEN` is not set, the application will:

1. **Display an authorization URL** in the terminal
2. **Start a local HTTP server** to handle the OAuth callback
3. **Wait for user approval** (browser-based consent screen)
4. **Exchange the authorization code for an access token**
5. **Print the token to the terminal** for easy copy-paste into `setup.ps1`

## Configuration Required

For desktop OAuth flow to work, you need to have the following already configured in `setup.ps1`:

```powershell
# These three must be set:
$env:GOOGLE_OAUTH_CLIENT_ID     = "XXX.apps.googleusercontent.com"
$env:GOOGLE_OAUTH_CLIENT_SECRET = "GOCSPX-..."
$env:GOOGLE_OAUTH_REDIRECT_URL  = "http://127.0.0.1:8787/oauth/callback"
```

**Important:** In Google Cloud Console, make sure to add `http://127.0.0.1:8787/oauth/callback` to your OAuth 2.0 application's **Authorized redirect URIs** list.

## First Run Setup

1. **Fill in the required values** in `setup.ps1`:
   - Telegram bot token and user/chat IDs
   - Copilot CLI path
   - Google OAuth client ID, secret, and redirect URL

2. **Run the setup**:
   ```powershell
   .\setup.ps1
   ```

3. **When the application starts**, if Google OAuth is configured:
   - You'll see a browser authorization URL in the terminal
   - A local HTTP server starts on `127.0.0.1:8787`
   - Open the URL in your browser and authorize access
   - The browser will redirect back to the local server
   - The authorization code is exchanged for an access token
   - **The token is printed to the terminal**

4. **Copy the token** from the terminal output:
   ```powershell
   $env:GOOGLE_OAUTH_ACCESS_TOKEN = "ya29.a0..."
   ```

5. **Update `setup.ps1`** with the token and re-run to persist it

## What Happens on Startup

The startup sequence is:
1. Load configuration from environment variables
2. **Check if Google OAuth needs interactive setup**
3. If needed, display auth URL and wait for user (non-blocking to app startup)
4. Build runtime tools (Google tools enabled if we got a token)
5. Start the assistant

If OAuth setup fails for any reason, the application logs a warning and continues with Google tools disabled.

## Scopes Requested

The default scopes for Google Workspace integration are:
- `https://www.googleapis.com/auth/gmail.modify` - Gmail access
- `https://www.googleapis.com/auth/calendar` - Google Calendar access
- `https://www.googleapis.com/auth/userinfo.email` - User email info

## Troubleshooting

**"oauth callback server: address already in use"**
- Another process is using port 8787
- Change the port in your redirect URL or stop the other process

**Authorization URL doesn't load**
- Verify your Google OAuth app is set to "Desktop application" type
- Check redirect URL matches exactly in Google Cloud Console

**Authorization code timeout**
- You have 5 minutes to complete the browser authorization
- Start over if you exceed the timeout

**Token exchange fails**
- Verify client secret is correct
- Ensure redirect URL matches exactly between setup and Google Console
