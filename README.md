6. Seeding Data

Upload CSV: /api/reconciliation/upload → creates batch and processes asynchronously.

Upload invoices only: /api/invoices/upload → inserts invoices without creating a batch.

CSV format should include:
InvoiceNumber,CustomerName,CustomerEmail,Amount,Status,DueDate,...

Technical Decisions
Matching Algorithm

Approach:
Exact amount match + fuzzy name similarity (based on shared words, normalized).
Date proximity adds small score adjustments.

Reason:
Balances accuracy with simplicity. Avoids overcomplicating with full Levenshtein or Jaro-Winkler.
Provides auto_matched, needs_review, unmatched categories.

Performance & Caching

Large CSV Processing: Processed asynchronously via Go routines (go processCSV). Batch progress updated every 100 rows.

//Caching: sync.Map used for progressCache and statsCache per batch to reduce repeated DB queries. Final batch state persisted to DB.

Background Jobs

Implementation: Goroutines for async processing.

Progress tracking: GetBatchProgress endpoint fetches progress from DB/cache.

Search

Invoice search: Filterable by invoice number, customer name, and status.

Indexing: DB indexes on InvoiceNumber and CustomerName recommended for performance.

Pagination

Strategy: Cursor-based pagination in ListTransactions.

Reason: Efficient for large datasets; avoids offset-based pagination pitfalls.

Trade-offs & Limitations
Improvements with more time: 
  - Clean bank transaction descriptions by removing noise words before matching, then apply advanced fuzzy matching algorithms (Jaro-Winkler, Levenshtein) for more accurate results.
  - Distributed CSV processing using queues for very large files.
  - Retry mechanisms for failed transaction matches.

Distributed CSV processing using queues for very large files.

Retry mechanisms for failed transaction matches.

Scaling limits:

Current implementation handles thousands of transactions efficiently.

Millions of transactions may require horizontal scaling, distributed workers, or message queues.

Known issues:

Duplicate CSV uploads may create duplicates.

Fuzzy matching may produce false positives in ambiguous cases.

Performance Results
Dataset Size	Processing Time	Notes
1,000 transactions	~2–3 seconds	Asynchronous, progress updates every 100 rows


API Endpoints
Health

GET /api/health → Check if server is running.

Reconciliation Batch

POST /api/reconciliation/upload → Upload CSV, create batch, process async

GET /api/reconciliation/:batchId → Get batch progress

GET /api/reconciliation/:batchId/transactions → List transactions (cursor-based pagination)

POST /api/reconciliation/:batchId/bulk-confirm → Confirm all auto-matched transactions

POST /api/reconciliation/invoice → Create a single invoice

Transactions

POST /api/transactions/:id/confirm → Confirm transaction

POST /api/transactions/:id/reject → Reject transaction

POST /api/transactions/:id/match → Manual match to invoice

POST /api/transactions/:id/external → Mark transaction as external

Invoice Upload

POST /api/invoices/upload → Upload invoices CSV without creating batch
