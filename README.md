## Seeding Data

- Upload a CSV of invoices using the `/api/invoices/upload` endpoint.  
- Upload a CSV of bank transactions using `/api/reconciliation/upload`.

---

## Technical Decisions

### Matching Algorithm
- **Exact amount match** first to narrow candidates.  
- **Fuzzy name matching** using tokenized Levenshtein similarity.  
- **Date proximity scoring** gives higher weight to transactions near invoice due date.  
- **Ambiguity penalty** reduces confidence when multiple candidate invoices match.  
- **Weighted confidence score formula**:  

finalScore = 0.6 * nameScore + 0.3 * dateScore + 0.1 * ambiguityScore


- **Transactions categorized**:
  - `auto_matched` ≥ 90  
  - `needs_review` ≥ 60  
  - `unmatched` otherwise  

### Caching Strategy
- Invoices loaded into **in-memory cache grouped by amount**.  
- Lookup during matching **does not hit DB repeatedly**.  
- Read-heavy, rarely changing data (invoice info) benefits from caching.

### Background Jobs
- CSV processing happens **asynchronously using goroutines**.  
- Each upload creates a batch and updates progress every 100 rows.  
- Frontend polls `/api/reconciliation/:batchID` for progress.

### Search & Pagination
- Transactions can be filtered by **status** (`all`, `auto_matched`, `needs_review`, etc.)  
- **Cursor-based pagination** efficiently fetches rows without loading all data in memory.

### Performance
- CSV rows processed in **streaming fashion**, no large in-memory accumulation.  
- Matching leverages **cached invoices** → reduces DB lookups.  
- Pagination + indexing ensures **search under 200ms** for typical batch sizes.

### Measured Processing Times:
- 1,000 transactions: ~1 second
- 10,000 transactions: ~9 seconds

### Trade-offs & Limitations
- Current implementation stores **all invoices in memory**; large datasets may require **Redis or distributed cache**.  
- Levenshtein similarity is **O(n*m)**; large strings may slow down matching.  
- **No undo** for bulk-confirm operations.  
- Frontend polling interval is fixed (1.5s) — could use **WebSocket** for real-time updates.  
- Currently assumes CSV date formats **DD-MM-YYYY** or **YYYY-MM-DD**.
