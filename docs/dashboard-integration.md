# Dashboard and local API integration

The dashboard uses a typed API client in `web/src/lib/api`. Browser code calls only a same-origin Next.js route at `/api/local/*`. That route forwards an allowlisted subset of `/api/v1/*` to a configured loopback Go server and adds the bearer token on the server side. It checks the configured dashboard Host and Origin, requires the dashboard marker header, limits request bodies, disables caching, and does not forward browser cookies. The token is never sent to browser JavaScript, local storage, or unrelated page scripts.

The proxy reads `LOCAL_CAPTURE_API_URL`, `LOCAL_CAPTURE_API_TOKEN`, and `LOCAL_CAPTURE_DASHBOARD_ORIGIN` from its server process environment. Do not prefix the token with `NEXT_PUBLIC_` or put it in a browser-readable file. The repository does not yet provide a trusted launcher or pairing flow to provision the token to a dashboard server process. Until that exists, a developer must provision it in a private process environment; this is a development integration and not a polished desktop connection flow.

Note content, lists, search results, and downloaded attachments are held only in page memory while the dashboard is open. The dashboard does not write capture data to local storage or IndexedDB. Download previews use temporary object URLs and are revoked when replaced or the page is cleared/unmounted. A locked/authentication response clears the current in-memory data. The current Go API has no vault status route, and this repository does not yet have an implemented encrypted vault; the UI therefore only displays a locked state if the API explicitly returns HTTP 423 or `vault_locked`.

## Current backend limits

The route contract documents collection list/update/delete, tag list/create/remove, attachment upload/download/delete, and reminder operations. Some of these handler methods are not backed by application services and persistence in the current repository, and there is no production composition root that wires the API server to a real store. Those calls will surface the backend's API error to the user; the dashboard does not synthesize or persist substitute data.

Attachment metadata and raw upload/download routes exist at the HTTP contract, but there is no encrypted attachment vault implementation. Do not use these routes for real private content until encrypted storage is implemented and reviewed. The current notes database also stores plaintext. This dashboard integration does not change those storage properties.

The auth token currently has broad configured scopes rather than a dashboard pairing grant workflow. The backend must bind to a literal loopback address and allow the exact dashboard origin in its origin configuration. The Next dashboard server should itself be reachable only from the local machine in the intended deployment. The proxy currently accepts only fixed route patterns and will not act as an arbitrary URL proxy.
