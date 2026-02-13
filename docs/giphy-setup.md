# Giphy Integration Setup

AmityVox supports GIF search and sending via the [Giphy API](https://developers.giphy.com/). GIFs are sent as regular messages containing a Giphy URL which is rendered inline.

## Getting a Giphy API Key

1. Go to [https://developers.giphy.com/dashboard/](https://developers.giphy.com/dashboard/)
2. Create an account or sign in
3. Click **Create an App**
4. Select **API** (not SDK)
5. Name your app (e.g. "AmityVox") and add a description
6. Copy the **API Key** shown on the dashboard

Giphy offers two tiers:
- **Free tier**: 100 requests/hour, 1,000 requests/day — sufficient for small instances
- **Production tier**: Apply when you need higher limits

## Configuration

Add your Giphy API key to your AmityVox configuration:

### Option 1: Config file (`amityvox.toml`)

```toml
[giphy]
enabled = true
api_key = "your-api-key-here"
```

### Option 2: Environment variables

```bash
AMITYVOX_GIPHY_ENABLED=true
AMITYVOX_GIPHY_API_KEY=your-api-key-here
```

### Option 3: Docker Compose

Add to your `docker-compose.yml` under the AmityVox service:

```yaml
services:
  amityvox:
    environment:
      - AMITYVOX_GIPHY_ENABLED=true
      - AMITYVOX_GIPHY_API_KEY=your-api-key-here
```

Then restart:

```bash
docker compose restart amityvox
```

## How It Works

1. The backend exposes proxy endpoints:
   - `GET /api/v1/giphy/search?q=query&limit=25` — search GIFs
   - `GET /api/v1/giphy/trending?limit=25` — trending GIFs
   - `GET /api/v1/giphy/categories?limit=15` — GIF categories with representative thumbnails
2. The backend calls Giphy's API with the server-side API key (never exposed to clients)
3. The frontend displays results in a GIF picker panel
4. When a user selects a GIF, the Giphy URL is sent as a message
5. GIF URLs render as inline images in the chat

## Content Rating

All GIF searches are filtered to **PG-13** rating by the backend proxy. This is hardcoded and cannot be overridden by clients.

## Disabling Giphy

Set `enabled = false` in the `[giphy]` config section, or remove the API key. When disabled:
- The GIF button is hidden from the message input
- The proxy endpoints return 503 Service Unavailable

## Privacy Note

When Giphy is enabled:
- Search queries are sent to Giphy's servers from your AmityVox backend (not from client browsers)
- GIF images are loaded from Giphy's CDN by clients when displayed
- Giphy may log requests according to their privacy policy

If privacy is a concern, leave Giphy disabled. Users can still paste GIF URLs manually.
