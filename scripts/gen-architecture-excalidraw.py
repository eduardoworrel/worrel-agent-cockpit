#!/usr/bin/env python3
"""Generator for the worrel-agent-cockpit architecture Excalidraw diagram."""

import json
import secrets
from pathlib import Path
from typing import List, Optional

OUT = Path(__file__).resolve().parent.parent / "docs" / "architecture.excalidraw"

# ---- helpers ---------------------------------------------------------------

_id_counter = 0


def nid(prefix: str) -> str:
    global _id_counter
    _id_counter += 1
    return f"{prefix}{_id_counter}"


def box(
    x: float,
    y: float,
    w: float,
    h: float,
    label: str,
    *,
    stroke: str = "#1e1e1e",
    bg: str = "#ffffff",
    fill_style: str = "solid",
    stroke_style: str = "solid",
    stroke_width: float = 1.5,
    font_size: int = 16,
    font_weight: int = 400,
    group: Optional[str] = None,
    text_color: str = "#1e1e1e",
    roundness: Optional[dict] = None,
    align: str = "center",
    container_fill: str = "solid",
) -> List[dict]:
    """Create a labelled rectangle (rect + text) and return both elements."""
    rid = nid("r")
    tid = nid("t")
    rect: dict = {
        "type": "rectangle",
        "id": rid,
        "x": x,
        "y": y,
        "width": w,
        "height": h,
        "angle": 0,
        "strokeColor": stroke,
        "backgroundColor": bg,
        "fillStyle": fill_style,
        "strokeWidth": stroke_width,
        "strokeStyle": stroke_style,
        "roughness": 1,
        "opacity": 100,
        "groupIds": [group] if group else [],
        "frameId": None,
        "roundness": roundness or {"type": 3},
        "seed": secrets.randbelow(2**31),
        "version": 1,
        "versionNonce": secrets.randbelow(2**31),
        "isDeleted": False,
        "boundElements": [{"type": "text", "id": tid}],
        "updated": 1,
        "link": None,
        "locked": False,
    }
    text: dict = {
        "type": "text",
        "id": tid,
        "x": x + 8,
        "y": y + (h - font_size - 8) / 2,
        "width": w - 16,
        "height": font_size + 8,
        "angle": 0,
        "strokeColor": text_color,
        "backgroundColor": "transparent",
        "fillStyle": "solid",
        "strokeWidth": 1,
        "strokeStyle": "solid",
        "roughness": 1,
        "opacity": 100,
        "groupIds": [group] if group else [],
        "frameId": None,
        "roundness": None,
        "seed": secrets.randbelow(2**31),
        "version": 1,
        "versionNonce": secrets.randbelow(2**31),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1,
        "link": None,
        "locked": False,
        "fontSize": font_size,
        "fontFamily": 1,
        "text": label,
        "rawText": label,
        "textAlign": align,
        "verticalAlign": "middle",
        "containerId": rid,
        "originalText": label,
        "lineHeight": 1.25,
        "baseline": int(font_size * 0.85),
    }
    return [rect, text]


def diamond(
    x: float,
    y: float,
    w: float,
    h: float,
    label: str,
    *,
    stroke: str = "#1e1e1e",
    bg: str = "#ffffff",
    group: Optional[str] = None,
    font_size: int = 14,
) -> List[dict]:
    rid = nid("d")
    tid = nid("t")
    d: dict = {
        "type": "diamond",
        "id": rid,
        "x": x,
        "y": y,
        "width": w,
        "height": h,
        "angle": 0,
        "strokeColor": stroke,
        "backgroundColor": bg,
        "fillStyle": "solid",
        "strokeWidth": 1.5,
        "strokeStyle": "solid",
        "roughness": 1,
        "opacity": 100,
        "groupIds": [group] if group else [],
        "frameId": None,
        "roundness": {"type": 3},
        "seed": secrets.randbelow(2**31),
        "version": 1,
        "versionNonce": secrets.randbelow(2**31),
        "isDeleted": False,
        "boundElements": [{"type": "text", "id": tid}],
        "updated": 1,
        "link": None,
        "locked": False,
    }
    t: dict = {
        "type": "text",
        "id": tid,
        "x": x + 8,
        "y": y + (h - font_size - 8) / 2,
        "width": w - 16,
        "height": font_size + 8,
        "angle": 0,
        "strokeColor": "#1e1e1e",
        "backgroundColor": "transparent",
        "fillStyle": "solid",
        "strokeWidth": 1,
        "strokeStyle": "solid",
        "roughness": 1,
        "opacity": 100,
        "groupIds": [group] if group else [],
        "frameId": None,
        "roundness": None,
        "seed": secrets.randbelow(2**31),
        "version": 1,
        "versionNonce": secrets.randbelow(2**31),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1,
        "link": None,
        "locked": False,
        "fontSize": font_size,
        "fontFamily": 1,
        "text": label,
        "rawText": label,
        "textAlign": "center",
        "verticalAlign": "middle",
        "containerId": rid,
        "originalText": label,
        "lineHeight": 1.25,
        "baseline": int(font_size * 0.85),
    }
    return [d, t]


def text(
    x: float,
    y: float,
    label: str,
    *,
    font_size: int = 18,
    color: str = "#1e1e1e",
    weight: int = 400,
    align: str = "left",
    w: Optional[int] = None,
    group: Optional[str] = None,
) -> dict:
    width = w or max(40, int(len(label) * font_size * 0.55) + 16)
    return {
        "type": "text",
        "id": nid("tx"),
        "x": x,
        "y": y,
        "width": width,
        "height": font_size + 8,
        "angle": 0,
        "strokeColor": color,
        "backgroundColor": "transparent",
        "fillStyle": "solid",
        "strokeWidth": 1,
        "strokeStyle": "solid",
        "roughness": 0,
        "opacity": 100,
        "groupIds": [group] if group else [],
        "frameId": None,
        "roundness": None,
        "seed": secrets.randbelow(2**31),
        "version": 1,
        "versionNonce": secrets.randbelow(2**31),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1,
        "link": None,
        "locked": False,
        "fontSize": font_size,
        "fontFamily": 1,
        "text": label,
        "rawText": label,
        "textAlign": align,
        "verticalAlign": "top",
        "containerId": None,
        "originalText": label,
        "lineHeight": 1.25,
        "baseline": int(font_size * 0.85),
    }


def arrow(
    x1: float,
    y1: float,
    x2: float,
    y2: float,
    *,
    stroke: str = "#1e1e1e",
    stroke_width: float = 1.5,
    stroke_style: str = "solid",
    label: Optional[str] = None,
    label_bg: str = "#ffffff",
    dashed: bool = False,
) -> List[dict]:
    elems: list[dict] = []
    style = "dashed" if dashed else stroke_style
    aid = nid("a")
    pts = [[0, 0], [x2 - x1, y2 - y1]]
    a: dict = {
        "type": "arrow",
        "id": aid,
        "x": x1,
        "y": y1,
        "width": abs(x2 - x1),
        "height": abs(y2 - y1),
        "angle": 0,
        "strokeColor": stroke,
        "backgroundColor": "transparent",
        "fillStyle": "solid",
        "strokeWidth": stroke_width,
        "strokeStyle": style,
        "roughness": 1,
        "opacity": 100,
        "groupIds": [],
        "frameId": None,
        "roundness": {"type": 2},
        "seed": secrets.randbelow(2**31),
        "version": 1,
        "versionNonce": secrets.randbelow(2**31),
        "isDeleted": False,
        "boundElements": None,
        "updated": 1,
        "link": None,
        "locked": False,
        "points": pts,
        "lastCommittedPoint": None,
        "startBinding": None,
        "endBinding": None,
        "startArrowhead": None,
        "endArrowhead": "arrow",
        "elbowed": False,
    }
    elems.append(a)
    if label:
        mid_x = (x1 + x2) / 2
        mid_y = (y1 + y2) / 2
        lw = max(60, int(len(label) * 9 + 16))
        lh = 22
        bg_id = nid("lbg")
        bg_rect: dict = {
            "type": "rectangle",
            "id": bg_id,
            "x": mid_x - lw / 2,
            "y": mid_y - lh / 2,
            "width": lw,
            "height": lh,
            "angle": 0,
            "strokeColor": "transparent",
            "backgroundColor": label_bg,
            "fillStyle": "solid",
            "strokeWidth": 1,
            "strokeStyle": "solid",
            "roughness": 0,
            "opacity": 100,
            "groupIds": [],
            "frameId": None,
            "roundness": None,
            "seed": secrets.randbelow(2**31),
            "version": 1,
            "versionNonce": secrets.randbelow(2**31),
            "isDeleted": False,
            "boundElements": None,
            "updated": 1,
            "link": None,
            "locked": False,
        }
        lt: dict = {
            "type": "text",
            "id": nid("lt"),
            "x": mid_x - lw / 2,
            "y": mid_y - lh / 2 + 2,
            "width": lw,
            "height": lh,
            "angle": 0,
            "strokeColor": "#1e1e1e",
            "backgroundColor": "transparent",
            "fillStyle": "solid",
            "strokeWidth": 1,
            "strokeStyle": "solid",
            "roughness": 0,
            "opacity": 100,
            "groupIds": [],
            "frameId": None,
            "roundness": None,
            "seed": secrets.randbelow(2**31),
            "version": 1,
            "versionNonce": secrets.randbelow(2**31),
            "isDeleted": False,
            "boundElements": None,
            "updated": 1,
            "link": None,
            "locked": False,
            "fontSize": 12,
            "fontFamily": 1,
            "text": label,
            "rawText": label,
            "textAlign": "center",
            "verticalAlign": "middle",
            "containerId": None,
            "originalText": label,
            "lineHeight": 1.25,
            "baseline": 11,
        }
        elems.extend([bg_rect, lt])
    return elems


# ---- canvas ----------------------------------------------------------------

elements: List[dict] = []

# Title
elements.append(
    text(
        320,
        20,
        "worrel — Agent Cockpit · Arquitetura de runtime",
        font_size=28,
        weight=600,
    )
)
elements.append(
    text(
        540,
        60,
        "Binário Go único + UI React embutida · SQLite local · zero rede em runtime",
        font_size=14,
        color="#495057",
    )
)

# ===== ZONE 1: Distribution =================================================
elements.append(
    text(60, 100, "1 · Distribuição (npm → binário Go)", font_size=18, color="#1971c2", weight=600)
)

# Boxes (single row)
y1 = 140
h1 = 70
xs = [
    (60, 200, "npx worrel@latest", "#d0ebff", "#1971c2"),
    (300, 200, "bin/worrel.js\n(shim Node)", "#d0ebff", "#1971c2"),
    (540, 220, "@worrel/{darwin-arm64, darwin-x64,\nlinux-x64, linux-arm64}", "#d0ebff", "#1971c2"),
    (810, 200, "./bin/worrel\n(Go 1.26 · single binary)", "#a5d8ff", "#1971c2"),
    (1060, 180, "HTTP em\n127.0.0.1:7717", "#fff3bf", "#fab005"),
    (1290, 180, "Browser aberto\n(--no-open desativa)", "#fff3bf", "#fab005"),
]
for x, w, lbl, bg, sc in xs:
    elements.extend(box(x, y1, w, h1, lbl, bg=bg, stroke=sc, group="dist"))

# Arrows between them
for (x1, w1, *_), (x2, w2, *_) in zip(xs[:-1], xs[1:]):
    elements.extend(arrow(x1 + w1, y1 + h1 / 2, x2, y1 + h1 / 2))

# Label under arrow to binary
elements.append(text(280, 220, "spawn stdio", font_size=12, color="#495057"))

# ===== ZONE 2: Web UI =======================================================
elements.append(
    text(60, 250, "2 · Web UI (React 19 · Vite · React Router · i18next · xterm.js)",
         font_size=18, color="#6741d9", weight=600)
)

# Outer "browser" container
ui_x, ui_y, ui_w, ui_h = 60, 290, 1830, 360
elements.extend(
    box(
        ui_x,
        ui_y,
        ui_w,
        ui_h,
        "",
        bg="#f3f0ff",
        stroke="#6741d9",
        stroke_style="dashed",
        stroke_width=1,
        roundness={"type": 3},
        font_size=1,
    )
)
# Tab/title bar
elements.append(text(ui_x + 12, ui_y + 8, "localhost:7717", font_size=12, color="#6741d9"))

# Sidebar
sb_x, sb_y, sb_w, sb_h = ui_x + 20, ui_y + 40, 220, 250
elements.extend(
    box(sb_x, sb_y, sb_w, sb_h, "", bg="#e5dbff", stroke="#6741d9", font_size=1)
)
elements.append(text(sb_x + 10, sb_y + 6, "Sidebar (NavLink)", font_size=12, color="#6741d9", weight=600))
nav_items = [
    "Dashboard",
    "Suggestions ●",
    "Sessions",
    "Retro",
    "Chat",
    "Pipelines",
    "Settings",
]
for i, item in enumerate(nav_items):
    elements.append(text(sb_x + 18, sb_y + 32 + i * 26, "▸ " + item, font_size=14, color="#1e1e1e"))

# New session button
elements.extend(
    box(sb_x + 14, sb_y + sb_h - 56, sb_w - 28, 40, "+ Nova sessão", bg="#6741d9", text_color="#ffffff", group="ui")
)

# Main area: routes
main_x, main_y, main_w, main_h = sb_x + sb_w + 20, sb_y, 1500, 250
elements.extend(box(main_x, main_y, main_w, main_h, "", bg="#ffffff", stroke="#6741d9", font_size=1))
elements.append(text(main_x + 10, main_y + 6, "<Routes>", font_size=12, color="#6741d9", weight=600))

# Route pages as small cards in a 4x2 grid
routes = [
    ("Dashboard", "useEvents (WS)"),
    ("Project", "por escopo"),
    ("Sessions", "lista/hub"),
    ("Terminal", "xterm.js + PTY"),
    ("Suggestions", "fila revisável"),
    ("Retro", "4 estágios"),
    ("Chat", "destilação"),
    ("Pipelines", "apply auto"),
]
card_w, card_h = 170, 60
for i, (name, sub) in enumerate(routes):
    cx = main_x + 20 + (i % 4) * (card_w + 14)
    cy = main_y + 36 + (i // 4) * (card_h + 16)
    elements.extend(
        box(
            cx,
            cy,
            card_w,
            card_h,
            f"{name}\n· {sub} ·",
            bg="#ede7f6",
            stroke="#6741d9",
            stroke_width=1,
            font_size=12,
        )
    )

# SessionTabs row
elements.extend(
    box(
        main_x + 20,
        main_y + main_h - 38,
        main_w - 40,
        28,
        "SessionTabs  (sessões ativas como abas)",
        bg="#f3f0ff",
        stroke="#6741d9",
        stroke_width=1,
        font_size=12,
    )
)

# Modals floating
elements.extend(
    box(
        ui_x + 440,
        ui_y + 230,
        280,
        80,
        "SecretApprovalModal\n(MCP: secret.approval_requested)",
        bg="#fff5f5",
        stroke="#c92a2a",
        font_size=12,
    )
)
elements.extend(
    box(
        ui_x + 760,
        ui_y + 230,
        260,
        80,
        "NewSessionModal\n(escolhe projeto + adapter)",
        bg="#fff9db",
        stroke="#fab005",
        font_size=12,
    )
)

# WebSocket arrow
elements.extend(
    arrow(
        ui_x + ui_w / 2,
        ui_y + ui_h,
        ui_x + ui_w / 2,
        ui_y + ui_h + 50,
        stroke="#6741d9",
        label="WS /api/events",
        label_bg="#f3f0ff",
    )
)

# ===== ZONE 3: Go Process (cockpit) =========================================
elements.append(
    text(
        60,
        690,
        "3 · Processo Go (cockpit) — cmd/worrel/main.go",
        font_size=18,
        color="#e8590c",
        weight=600,
    )
)

# Outer "process" container
p_x, p_y, p_w, p_h = 60, 730, 1830, 380
elements.extend(
    box(
        p_x,
        p_y,
        p_w,
        p_h,
        "",
        bg="#fff4e6",
        stroke="#e8590c",
        stroke_style="dashed",
        stroke_width=1,
        font_size=1,
    )
)
elements.append(
    text(p_x + 12, p_y + 8, "main.go — composição por injeção de dependências",
         font_size=12, color="#e8590c", weight=600)
)

# Columns inside the process
col_w = 350
col_h = 310
col_y = p_y + 40
gaps = [p_x + 20, p_x + 390, p_x + 760, p_x + 1130, p_x + 1450]
col_titles = [
    "HTTP / MCP",
    "Núcleo / Estado",
    "Motores",
    "Persistência",
    "Aprovação humana",
]
col_bgs = [
    "#ffe8cc",
    "#ffe8cc",
    "#ffe8cc",
    "#ffe8cc",
    "#ffe8cc",
]
for cx, title in zip(gaps, col_titles):
    elements.extend(
        box(
            cx,
            col_y,
            col_w,
            col_h,
            "",
            bg="#fffaf0",
            stroke="#e8590c",
            stroke_width=1.2,
            font_size=1,
        )
    )
    elements.append(text(cx + 10, col_y + 6, title, font_size=14, color="#e8590c", weight=600))

# Column 1: HTTP / MCP
c1 = gaps[0]
elements.extend(box(c1 + 14, col_y + 38, col_w - 28, 56, "httpapi\nREST + WebSocket +\nserve UI embutida", bg="#ffd8a8", stroke="#e8590c", font_size=12))
elements.extend(box(c1 + 14, col_y + 104, col_w - 28, 56, "mcpserver\ntools p/ agentes:\nlist_projects, load_memory,\nskills, report_event…", bg="#ffd8a8", stroke="#e8590c", font_size=12))

# Column 2: Core / State
c2 = gaps[1]
elements.extend(box(c2 + 14, col_y + 38, col_w - 28, 56, "bus\n(event bus interno)", bg="#ffd8a8", stroke="#e8590c", font_size=12))
elements.extend(box(c2 + 14, col_y + 104, col_w - 28, 56, "workspace\nManager: projetos,\nsettings, MCP token", bg="#ffd8a8", stroke="#e8590c", font_size=12))
elements.extend(box(c2 + 14, col_y + 170, col_w - 28, 56, "adapter.Registry\nclaudecode, opencode,\ngemini, codex, pidev", bg="#ffe066", stroke="#fab005", font_size=12))

# Column 3: Engines
c3 = gaps[2]
eng_items = [
    ("distill", "Engine + Importer +\nWatcher (fsnotify) +\nScreening 2 fases"),
    ("apply", "Applier + auto-apply\n(opt-in por skill)"),
    ("skillpkg", "Evolução: learned /\ncorrection / variant\n+ lineage + health"),
    ("retro", "4 estágios:\nInventory → Scope →\nCluster → Distill"),
    ("handoff", "Generator + Summarizer\n(perto de 80% do ctx)"),
    ("chat", "Service: sugestões\norigin=chat"),
    ("wrapper", "PTY spawner\n(BuildInteractive)"),
    ("retention", "Janitor: varre\nexpirados a cada 6h"),
]
for i, (name, sub) in enumerate(eng_items):
    yy = col_y + 38 + (i // 2) * 65
    xx = c3 + 14 + (i % 2) * ((col_w - 28) / 2 + 4)
    ww = (col_w - 28) / 2 - 2
    elements.extend(box(xx, yy, ww, 56, f"{name}\n{sub}", bg="#ffc9c9", stroke="#c92a2a", stroke_width=1, font_size=10))

# Column 4: Persistence
c4 = gaps[3]
elements.extend(box(c4 + 14, col_y + 38, col_w - 28, 80, "store\nSQLite (modernc)\nprojects · sessions ·\nsuggestions · memories ·\nskills · secrets · settings", bg="#b2f2bb", stroke="#2f9e44", font_size=11))
elements.extend(box(c4 + 14, col_y + 128, col_w - 28, 80, "vault\nAES-256-GCM + Keychain\nmodo valor / modo receita\nenv opt-in · auditoria", bg="#b2f2bb", stroke="#2f9e44", font_size=11))
elements.extend(box(c4 + 14, col_y + 218, col_w - 28, 56, "mirror\n(transcripts copiados\npara ~/.worrel)", bg="#b2f2bb", stroke="#2f9e44", font_size=11))

# Column 5: Approval
c5 = gaps[4]
elements.extend(
    box(
        c5 + 14,
        col_y + 38,
        col_w - 28,
        80,
        "Fila de sugestões\nnada vira artefato\nsem aprovação",
        bg="#fff5f5",
        stroke="#c92a2a",
        font_size=13,
        text_color="#c92a2a",
    )
)
elements.extend(
    box(
        c5 + 14,
        col_y + 128,
        col_w - 28,
        80,
        "Auditoria\npor acesso a segredo\n+ retenção configurável\n(padrão 30d)",
        bg="#fff5f5",
        stroke="#c92a2a",
        font_size=13,
        text_color="#c92a2a",
    )
)
elements.extend(
    box(
        c5 + 14,
        col_y + 218,
        col_w - 28,
        56,
        "Handoff\nperto de 80% do ctx:\nnova sessão com\nresumo estruturado",
        bg="#fff9db",
        stroke="#fab005",
        font_size=11,
    )
)

# Connection from process to data sources below
elements.extend(
    arrow(
        p_x + 200,
        p_y + p_h,
        p_x + 200,
        p_y + p_h + 50,
        stroke="#e8590c",
        label="Watcher (fsnotify)",
        label_bg="#fff4e6",
    )
)

# ===== ZONE 4: External =====================================================
elements.append(
    text(
        60,
        1170,
        "4 · Fontes externas (transcripts, processos CLI, LLMs via assinatura do usuário)",
        font_size=18,
        color="#495057",
        weight=600,
    )
)

ext_y = 1210
ext_h = 80
ext_items = [
    (60, 260, "~/.claude/projects\n~/.opencode/  ~/.gemini/…", "transcripts no disco\n(importer + watcher)", "#fab005"),
    (360, 240, "wrapper → adapter.BuildInteractive\n→ PTY", "processos CLI\n(Claude Code, OpenCode…)", "#868e96"),
    (640, 240, "Subscription LLM APIs\n(Anthropic, OpenAI, Google…)", "via CLIs do usuário\nWorrel NÃO tem API key", "#868e96"),
    (920, 240, "Keychain (macOS/Linux)\n+ WORREL_MASTER_PASSWORD", "cofre de segredos\n(opt-in valor)", "#868e96"),
    (1200, 240, "navegador do usuário\n@ http://127.0.0.1:7717", "cliente HTTP/WS\nda UI React", "#6741d9"),
    (1480, 310, "Qualquer CLI agêntico\n(mesmo fora do cockpit)\n→ importer importa histórico", "retro analysis:\n4 estágios sob demanda", "#6741d9"),
]
for x, w, lbl, sub, color in ext_items:
    elements.extend(
        box(x, ext_y, w, ext_h, "", bg="#f8f9fa", stroke=color, stroke_style="dashed", stroke_width=1.2, font_size=1)
    )
    elements.append(text(x + 10, ext_y + 6, lbl, font_size=12, color=color, weight=600))
    elements.append(text(x + 10, ext_y + 44, sub, font_size=11, color="#495057"))

# Internal flow arrows (data path)
# Watcher → importer → engine
elements.extend(arrow(p_x + 60, p_y + 50, p_x + 60, p_y + 50, stroke="#1e1e1e"))  # spacer to keep ordering

# Numbered legend on the right (data flow)
legend_x = 1480
legend_y = 290
elements.extend(
    box(legend_x, legend_y, 410, 360, "", bg="#f8f9fa", stroke="#1e1e1e", stroke_width=1, font_size=1)
)
elements.append(
    text(legend_x + 12, legend_y + 8, "Caminho do dado (sessão → memória)", font_size=14, weight=600)
)
steps = [
    "1. Agente roda (dentro ou fora) → transcripts no disco",
    "2. Watcher (fsnotify) ou boot → Importer",
    "3. Importer espelha → mirror + grava sessões no store",
    "4. Engine varre: screening local (heurística) + LLM headless",
    "5. Sugestões tipadas (memories, skills, projects)",
    "6. Fila revisável: usuário aprova (UI) ou MCP reporta",
    "7. apply → grava no store / skillpkg (com lineage) / mirror",
    "8. Próxima sessão: primer injeta memória + skills",
    "9. ~80% do contexto → handoff gera resumo estruturado",
    "10. Retenção: janitor poda transcripts brutos (30d)",
]
for i, s in enumerate(steps):
    elements.append(text(legend_x + 12, legend_y + 40 + i * 30, s, font_size=12, color="#1e1e1e"))

# Footer
elements.append(
    text(
        60,
        1320,
        "Cores: azul = distribuição · violeta = UI · laranja = núcleo Go · vermelho = motores · verde = persistência · cinza = externo",
        font_size=12,
        color="#868e96",
    )
)
elements.append(
    text(
        60,
        1345,
        "Garantias: 100% local · zero telemetria · sem API key própria · SQLite + AES-256-GCM · binário único Go com UI React embutida",
        font_size=12,
        color="#868e96",
    )
)

# ---- output ----------------------------------------------------------------

scene = {
    "type": "excalidraw",
    "version": 2,
    "source": "https://excalidraw.com",
    "elements": elements,
    "appState": {
        "gridSize": None,
        "viewBackgroundColor": "#ffffff",
    },
    "files": {},
}

OUT.write_text(json.dumps(scene, ensure_ascii=False, indent=2))
print(f"wrote {OUT} with {len(elements)} elements")
