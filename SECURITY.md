# Security

Do not publish credentials, subscription URLs, user databases or browsing/audit logs in issues. Use private contact with the repository owner for sensitive reports; if no private channel is available, open an issue describing only the issue class and request contact.

The preview uses bcrypt passwords, random hashed sessions, role checks, scoped hashed node credentials, origin checks, bounded request bodies and transactional report deduplication. These controls do not make an untested deployment production-ready. Use HTTPS and isolate PostgreSQL/Redis; keep backups private.

Old XBoard report requests have no durable batch identity. This is a known data-integrity limitation, explicitly labeled legacy, not a claim of exact-once accounting. Destination auditing is not part of panel traffic billing.
