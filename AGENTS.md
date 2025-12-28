# AI Agent Context Index for Koolo

This document provides a high-level architectural overview and context index for the Koolo Diablo II: Resurrected bot. It is designed to help AI agents understand the codebase structure, key components, and control flow to effectively modify or extend the application.

## 1. Project Overview

*   **Language**: Go (Golang)
*   **Purpose**: External bot for Diablo II: Resurrected (D2R).
*   **Mechanism**: Reads game memory (using `d2go` dependency) to fetch state and injects hardware events (Mouse/Keyboard) to interact. It does **not** inject code into the game process (mostly), relying on external control.
*   **Entry Point**: `cmd/koolo/main.go`

## 2. Architecture & Data Flow

The application follows a layered architecture:

1.  **Presentation Layer (`cmd/koolo`, `internal/server`)**:
    *   `main.go` bootstraps the app, logger, and config.
    *   `internal/server` runs an HTTP server and WebSocket hub to serve the web UI (HTML templates in `internal/server/templates`).
    *   The UI displays real-time status, logs, and manages configuration.

2.  **Orchestration Layer (`internal/bot`)**:
    *   **`SupervisorManager`**: Singleton that manages multiple `Supervisor` instances. Handles starting, stopping, and recovering crashed game clients.
    *   **`Supervisor`** (`SinglePlayerSupervisor`): Manages the lifecycle of a *single* bot instance/game window. It handles the "Out of Game" logic (menus, creating games, recovering from errors).
    *   **`Bot`**: The "In Game" loop. Once a game is joined, the `Supervisor` hands control to the `Bot`. The `Bot` executes the configured `Runs`.

3.  **Logic Layer (`internal/action`, `internal/run`, `internal/character`)**:
    *   **`Runs`**: High-level scripts (e.g., `Mephisto`, `Countess`, `Leveling`). They define *what* to do.
    *   **`Actions`**: Reusable behaviors composed of steps (e.g., `MoveTo`, `Interact`, `KillMonster`, `Buff`).
    *   **`Steps`**: Atomic operations (e.g., `Click`, `PressKey`).
    *   **`Character`**: Class-specific logic (Sorceress, Paladin, etc.) defining combat rotations (`KillMonsterSequence`), buffs, and keybindings.

4.  **Game Interface Layer (`internal/game`, `internal/pather`)**:
    *   **`MemoryReader`**: Reads data from the D2R process (HP, Mana, Position, Map Data). Wraps `d2go`.
    *   **`HID`**: Human Interface Device. Abstraction for clicking and typing.
    *   **`PathFinder`**: A* implementation using collision grids fetched from memory to navigate game areas.

5.  **Data Layer (`internal/config`, `internal/game/data.go`)**:
    *   **`Context`**: A shared object passed through the entire stack containing references to `Data`, `Logger`, `CharacterCfg`, and `GameReader`.
    *   **`Data`**: Snapshot of the current game state (Player, Monsters, Items, Map). Refreshed frequently in the game loop.

## 3. Key Subsystems

### 3.1 Map Data Service (`internal/game/map_client`)
Koolo does not generate map collision data purely in Go. It relies on an external executable `tools/koolo-map.exe` (which likely wraps a JS/C++ library for map generation).
*   **Mechanism**: `map_client.GetMapData` spawns `koolo-map.exe` as a subprocess with the current seed and difficulty.
*   **Output**: The subprocess outputs JSON representing the level layout (rooms, objects, NPCs) and collision grid.
*   **Integration**: The Go code parses this JSON into a 2D boolean grid used by the `PathFinder`.

### 3.2 Event Bus System (`internal/event`)
Koolo uses a centralized, channel-based event bus for decoupling logic from reporting (logging, Discord, Telegram).
*   **Core**: `internal/event/listener.go` manages a channel `events`.
*   **Emission**: Any part of the code can call `event.Send(e)`.
*   **Handling**: `Listener` iterates over registered `Handler` functions.
*   **Handlers**:
    *   `DiscordBot` / `TelegramBot`: Forward events to chat.
    *   `StatsHandler`: Updates in-memory stats for the UI.
    *   `DropLog`: Logs item drops to disk.
*   **Screenshotting**: Events can optionally carry an image (e.g., on error), which the listener saves to disk if debug mode is on.

## 4. Directory Structure (Annotated)

```text
koolo/
├── cmd/
│   └── koolo/           # Entry point (main.go)
├── config/              # Configuration files (YAML) & templates (pickits)
├── internal/
│   ├── action/          # Atomic game actions (Move, Interact, Buff, Town routines)
│   │   └── step/        # Lowest level steps (Click, Wait)
│   ├── bot/             # Core loop, SupervisorManager, and Scheduler
│   ├── character/       # Class implementations (Sorceress, Paladin, etc.)
│   ├── config/          # Configuration structs and loading logic
│   ├── container/       # (Legacy/Deprecation warning)
│   ├── context/         # Shared Context object definition
│   ├── drop/            # Drop logging and handling logic
│   ├── event/           # Event bus (log events, discord notifications)
│   ├── game/            # Game process interaction (MemoryReader, HID, Manager)
│   │   └── map_client/  # Client for fetching map collision data
│   ├── health/          # Health/Mana monitoring and Potion logic
│   ├── helper/          # General utils
│   ├── mule/            # Muling logic (stashing items to transfer)
│   ├── packet/          # (Experimental) Packet manipulation logic
│   ├── pather/          # Pathfinding algorithms (A*)
│   ├── pickit/          # Item evaluation rules (NIP format)
│   ├── remote/          # Discord/Telegram integrations
│   ├── run/             # Specific run scripts (Mephisto, Baal, etc.)
│   ├── server/          # Web server, API, and UI templates
│   ├── town/            # NPC interaction logic for each Act (Refill, Repair)
│   ├── ui/              # Coordinate mapping for Inventory/Stash grids
│   └── utils/           # System utilities (Sleep, Window handling)
└── tools/               # Helper binaries (map server, handle closer)
```

## 5. Key Concepts & Patterns

### The `Context` Object
Almost every function in the Logic and Game layers takes or accesses the `context.Context` (often wrapped in a struct). It holds:
*   `Data`: The latest game state.
*   `Logger`: For structured logging.
*   `CharacterCfg`: Configuration for the current character.
*   `PathFinder`, `HID`, `GameReader`.

### The Action/Step Pattern
Complex behaviors are built via composition:
1.  **Run**: "Do Mephisto"
    *   Calls `action.MoveTo(MephistoArea)`
    *   Calls `action.ClearArea(...)`
    *   Calls `character.KillMephisto()`
2.  **Action**: `MoveTo`
    *   Calculates path.
    *   Loops through waypoints.
    *   Executes `step.MoveTo(nextCoordinate)`.

### Memory Reading & Caching
The bot relies on `internal/game/memory_reader.go` to poll the game state.
*   **Map Data**: Fetched once per area entry (collisions are static for the session).
*   **Dynamic Data**: (Health, Monsters, Items) Refreshed constantly in the `Bot` loop (`internal/bot/bot.go`).

### Coordinates
*   **World Coordinates**: Used for movement and map navigation (X, Y in game world).
*   **Screen Coordinates**: Used for clicking items/UIs. `internal/ui` maps inventory grid slots to screen X/Y pixels. Note the difference between "Classic" (Legacy) and "Resurrected" graphics modes; the bot supports both but coordinate offsets differ.

## 6. Extension Guides

### How to add a new Run
1.  Create a new file in `internal/run/` (e.g., `new_boss.go`).
2.  Implement the `Run` interface (Name, Run).
3.  Register the run in `internal/config/runs.go` and `internal/run/run.go` (`BuildRuns` factory).
4.  Add configuration options in `internal/config/game_settings.go`.

### How to add a new Character Build
1.  Create a new file in `internal/character/` (e.g., `fire_sorc.go`).
2.  Implement the `Character` interface (Combat logic, Buffs, Keybindings).
3.  Register the build in `internal/character/character.go` (`BuildCharacter` factory).
4.  Add the class config options in `config/character_settings.go`.

### How to Fix an Interaction Bug
1.  Identify if it's an **Action** failure (logic) or **Step** failure (input/coordinates).
2.  Check `internal/ui` if mouse clicks are missing UI elements.
3.  Check `internal/game/hid.go` for input injection issues.
4.  Check `internal/pather` if the bot is getting stuck or failing to navigate.

## 7. Critical Considerations
*   **Safety**: The bot reads memory. Offsets change with D2R patches. Ensure `d2go` dependency is compatible with the current game version.
*   **Concurrency**: The `SupervisorManager` runs multiple bots. The `Bot` loop runs multiple goroutines (Health check, Game Data Refresh, Main Logic). Be careful with shared state in `Context`.
*   **Error Handling**: runs return errors. The `Bot` catches them. `ErrDied` triggers specific death handling. Other errors may trigger a game exit and restart (Chicken).
*   **External Deps**: `koolo-map.exe` must be present in `tools/` for pathfinding to work.
