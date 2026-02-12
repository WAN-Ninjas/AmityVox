# Giphy Integration Setup

AmityVox supports GIF search and sending via the Giphy API. GIFs are sent as regular messages containing a Giphy URL which is rendered inline.

## Getting a Giphy API Key

1. Go to [https://developers.giphy.com/](https://developers.giphy.com/)
2. Create an account or sign in
3. Click **Create an App**
4. Select **API** (not SDK)
5. Name your app (e.g. "AmityVox") and add a description
6. Copy the **API Key** shown on the dashboard

Giphy offers two tiers:
- **Free tier**: 100 requests/hour, 1000 requests/day — sufficient for small instances
- **Production tier**: Apply when you need higher limits

## Configuration

Add your Giphy API key to your AmityVox configuration:

### Option 1: Config file (`amityvox.toml`)

```toml
[integrations]
giphy_api_key = "your-api-key-here"
```

### Option 2: Environment variable

```bash
AMITYVOX_GIPHY_API_KEY=your-api-key-here
```

### Option 3: Docker Compose (`.env` file)

Add to your `.env` file:
```
GIPHY_API_KEY=your-api-key-here
```

And reference in `docker-compose.yml`:
```yaml
services:
  amityvox:
    environment:
      - AMITYVOX_GIPHY_API_KEY=${GIPHY_API_KEY}
```

## How It Works

1. The backend exposes a proxy endpoint: `GET /api/v1/giphy/search?q=query&limit=25`
2. The backend calls Giphy's API with the server-side API key (never exposed to clients)
3. The frontend displays results in a GIF picker panel
4. When a user selects a GIF, the Giphy URL is sent as a message
5. The message renderer detects Giphy URLs and renders them as inline GIF images

## Disabling Giphy

If no API key is configured, the GIF button is hidden from the message input and the proxy endpoint returns 404. No Giphy-related requests are made.

## Privacy Note

When Giphy is enabled:
- Search queries are sent to Giphy's servers from your AmityVox backend
- GIF images are loaded from Giphy's CDN by clients
- Giphy may log requests according to their privacy policy

If privacy is a concern, leave Giphy disabled. Users can still paste GIF URLs manually.
