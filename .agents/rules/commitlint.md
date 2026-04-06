# commitlint Rule

Trigger: On git commit or PR event

Purpose:
- Enforce commit message standards as per commitlint

Conditions:
- Trigger on new commit or PR creation

Actions:
- Run commitlint CLI against commit message
- If commit message fails, flag as non-compliant
- Notify agent or user for correction
- Block merge or commit if possible

---
