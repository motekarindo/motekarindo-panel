# Git Workflow

Motekar Panel uses small, verifiable commits.

## Commit Rule

After each completed module or coherent implementation slice:

1. Run safe verification:

   ```bash
   make test
   make build
   ```

2. Review changed files.
3. Commit only files related to the completed slice.
4. Use a conventional commit message.

## Commit Message Format

```text
<type>: <short imperative summary>

<optional body explaining why>
```

Common types:

- `feat`: new product functionality
- `fix`: bug fix
- `docs`: documentation-only change
- `test`: test-only change
- `refactor`: behavior-preserving code change
- `chore`: tooling, repository maintenance, generated support files

Examples:

```text
feat: scaffold panel and agent services
docs: document local API workflow
test: add nginx config golden tests
chore: add build commands
```

## Safety

Do not commit `.agents/`, local caches, built binaries, credentials, machine-specific config, or disposable test artifacts.

System tests that affect the OS are not run on the host machine. See [system-test-safety.md](system-test-safety.md).

