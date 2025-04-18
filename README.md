# Windsurf Memory Server v2

At present the Windsurf (previously Codeium) plugin in JetBrains IDE is super limited, as it can't access saved
rules nor memories like the not-real-great "Cascade" memory system in the main Windsurf IDE.

That Cascade system isn't very good anyway, so running your own simple local memory server instead works much
better anyway even in the Windsurf IDE.

That's what this project is.  Have it running in the background while you use the Windsurf AI agent, and every now
and then tell it to update the memories in the memory server with any new important lessons learned or information.

When you start a new AI agent session, you just tell it to fetch the latest memories from the memory server (using
curl):

```
Using curl, load the memories from the local memory server at 'http://localhost:8080/list-memories-by-tag?tag=memory_server'.
Remember to quote the url, otherwise the shell can be confused by the ? character in that url.
```

The tag bit is important if you have multiple projects using the same memory server.  That way you can keep the
memories separate.

ie:

* http://localhost:8080/list-memories-by-tag?tag=memory_server
* http://localhost:8080/list-memories-by-tag?tag=some_other_project
* etc

## Features
- Store, update, and archive versioned memories with tags
- Retrieve memories by ID, search term, or tag
- Project-specific filtering using tags (e.g., `memory_server`)
- REST API built with Go (Fuego framework) and SQLite
- VueJS 3 frontend served at the root endpoint
- Comprehensive automated test suite

## Quick Start

### Prerequisites

- Go (1.18+ recommended)
- SQLite3

### Clone and Run

```sh
$ git clone https://github.com/yourusername/windsurf_memory_server_v2
$ cd windsurf_memory_server_v2

# Start the server (default port 8080)
$ go run backend/main.go
```

The server will create a SQLite database at `~/Databases/memory_server.sqlite` by default.

### API Endpoints
- `POST   /save-memory` — Save a new memory version
- `POST   /update-memory` — Archive current and save new version
- `POST   /delete-memory` — Archive all versions of a memory
- `GET    /list-memories` — List all latest, non-archived memories
- `GET    /list-memories-by-tag?tag=your_tag` — List memories with a specific tag
- `GET    /get-memory-by-id/{memory_id}` — Get latest version by ID
- `GET    /search-memories?q=search_term` — Search memories by ID/content

### Updating Memories via curl

To update a memory, have the agent save it in JSON format to a file and use:
```sh
curl -X POST -H "Content-Type: application/json" --data-binary @update.json http://localhost:8080/update-memory
```

This avoids shell escaping issues.

### Running Tests

The test suite covers all major endpoints and behaviours. To run:

```sh
go test ./test/...
```

## Project Tagging

To support multi-project use, tag project-specific memories (e.g., `memory_server`). Use `/list-memories-by-tag` to filter accordingly.

## License

MIT.

Note that the code in this repo was written by Windsurf AI's (heh), so who knows what the actual legal status of this
code is.
