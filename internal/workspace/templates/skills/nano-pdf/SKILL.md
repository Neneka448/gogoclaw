---
name: nano-pdf
description: Edit PDFs with natural-language instructions using the nano-pdf CLI.
homepage: https://pypi.org/project/nano-pdf/
---

# nano-pdf

Use `nano-pdf` to apply edits to a specific page in a PDF using a natural-language instruction.

## Environment Check (MUST run first)

Before any PDF operation, verify `nano-pdf` is available:

```bash
#!/bin/bash
MISSING=()

if ! command -v nano-pdf &>/dev/null; then
  MISSING+=("nano-pdf")
fi

if [ ${#MISSING[@]} -eq 0 ]; then
  echo "ENV_OK"
else
  echo "ENV_MISSING: ${MISSING[*]}"
fi
```

**If `ENV_OK`**: proceed with the PDF task.

**If `ENV_MISSING`**: tell the user `nano-pdf` is not installed. Ask if they want to install it:

```bash
# Via uv (recommended)
uv tool install nano-pdf

# Via pip
pip install nano-pdf

# Via pipx
pipx install nano-pdf
```

Do NOT install automatically — always get user confirmation first. If the user declines, suggest alternative approaches (e.g., using Python PDF libraries directly).

---

## Quick start

```bash
nano-pdf edit deck.pdf 1 "Change the title to 'Q3 Results' and fix the typo in the subtitle"
```

Notes:

- Page numbers are 0-based or 1-based depending on the tool’s version/config; if the result looks off by one, retry with the other.
- Always sanity-check the output PDF before sending it out.