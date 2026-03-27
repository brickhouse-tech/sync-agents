---
trigger: always_on
---

# No Secrets

Never commit secrets, credentials, or sensitive values to the repository.

## Rules

- Do not hardcode API keys, tokens, passwords, or connection strings in source code
- Do not commit `.env`, `.envrc`, or any file containing secrets
- Use environment variables or a secrets manager (1Password, Vault, AWS Secrets Manager) for all credentials
- If a secret is accidentally committed, rotate it immediately — git history is permanent
- Use placeholder values in examples and documentation (e.g. `YOUR_API_KEY_HERE`)
- Ensure `.gitignore` includes: `.env`, `.env.*`, `*.pem`, `*.key`, `credentials.json`

## Detection

Flag any of these patterns in code review:
- Strings matching `sk-`, `ghp_`, `gho_`, `AKIA`, `xox`, `Bearer ` followed by a long alphanumeric string
- Base64-encoded blobs assigned to variables named `key`, `secret`, `token`, `password`, `credential`
- Hardcoded URLs containing `@` (embedded credentials)
