# Portfolio Demo: Portfolios, Projects & Milestones

This demo shows how an LLM agent (or human) can use mcli's generic commands to:

1. **Create boards** with typed columns, one group per portfolio
2. **Define a semantic layer** of saved queries/mutations with business-domain names
3. **Seed portfolios and projects** using the structured item commands
4. **Track milestones as subitems**, including typing the subitems board mcli hands back
5. **Roll up** — projects per portfolio, at-risk projects, milestone status

## Prerequisites

- `mcli auth login` (or `MONDAY_API_TOKEN` set)
- `jq` in PATH

Run the full demo: `./examples/portfolio-demo.sh`

---

## Phase 1: Create Boards

```sh
mcli board create --name "Portfolios" --kind public --empty   # → {"id":"..."}
mcli board create --name "Projects"   --kind public --empty
```

Each board starts empty (no default columns/items).

Projects get one group per portfolio, so "the projects in this portfolio" is a
native, cheap query (`item list --group`) rather than a client-side filter:

```sh
mcli board group create --board $PROJECTS_BOARD --name "Platform Modernization"
mcli board group create --board $PROJECTS_BOARD --name "Customer Experience"
```

Milestones are **subitems of projects** — monday materialises them on a
"Subitems of Projects" board that it creates on first use. Phase 5 shows how to
discover that board's ID and give it real columns.

## Phase 2: Define Columns

**Portfolios** — item name is the portfolio name:

```sh
mcli board column create --board $PORTFOLIOS_BOARD --title "Owner"  --type people
mcli board column create --board $PORTFOLIOS_BOARD --title "Budget" --type numbers
mcli board column create --board $PORTFOLIOS_BOARD --title "Health" --type status \
  --defaults '{"labels":{"0":"On Track","1":"At Risk","2":"Off Track"}}'
```

**Projects** — item name is the project name; `Portfolio` holds the parent
portfolio's name, which is the join key back to the Portfolios board:

```sh
mcli board column create --board $PROJECTS_BOARD --title "Portfolio" --type text
mcli board column create --board $PROJECTS_BOARD --title "Owner"     --type people
mcli board column create --board $PROJECTS_BOARD --title "Start"     --type date
mcli board column create --board $PROJECTS_BOARD --title "Target"    --type date
mcli board column create --board $PROJECTS_BOARD --title "Complete"  --type numbers
mcli board column create --board $PROJECTS_BOARD --title "Steering Approved" --type checkbox
mcli board column create --board $PROJECTS_BOARD --title "Risk Notes" --type long_text
mcli board column create --board $PROJECTS_BOARD --title "RAG" --type status \
  --defaults '{"labels":{"0":"Green","1":"Amber","2":"Red"}}'
```

Describe the columns whose meaning is not obvious from the title:

```sh
mcli board column describe --board $PROJECTS_BOARD --column $PROJ_RAG \
  --text "Delivery health: Green = on plan, Amber = recoverable slip, Red = escalate"
```

A `timeline` column (`--type timeline`) is also available and mcli decodes it as
`{"from":"…","to":"…"}`; this demo uses two `date` columns (Start / Target)
because their write shape — `{"date":"YYYY-MM-DD"}` — is the one documented for
`--col`.

## Phase 3: Semantic Layer

Saved queries and mutations give business-level names to operations. An LLM can
call `mcli mutation run set_project_health --var ...` without knowing GraphQL:

### Queries

| Name | Purpose |
|------|---------|
| `list_portfolios` | All portfolios with column values |
| `list_projects` | All projects with column values and milestone subitems |
| `projects_in_portfolio` | Projects in one portfolio group (the rollup query) |
| `get_project` | Single project detail with milestone column values |

```sh
mcli query save projects_in_portfolio --query \
  'query($boardId: ID!, $groupIds: [String!]) { boards(ids: [$boardId]) { groups(ids: $groupIds) { id title items_page(limit:100) { items { id name column_values { id text value } subitems { id name } } } } } }'
```

### Mutations

| Name | Purpose |
|------|---------|
| `create_portfolio` | Add a portfolio |
| `create_project` | Add a project into a portfolio group |
| `set_project_health` | Update RAG / percent complete / notes |
| `set_portfolio_health` | Roll the portfolio's own health status up |
| `add_milestone` | Add a milestone subitem to a project |

```sh
mcli mutation save add_milestone --query \
  'mutation($parent: ID!, $name: String!, $cols: JSON!) { create_subitem(parent_item_id: $parent, item_name: $name, column_values: $cols) { id } }'
```

## Phase 4: Seed Portfolios & Projects

Using the structured `item create` command (more ergonomic for setup). A typed
shorthand addresses the board's *single* column of that type, so Health (`status`)
and Budget (`numbers`) need no column ID; Owner (`people`) has no shorthand and uses
monday's raw write shape:

```sh
ME_ID=$(mcli me | jq -r '.id')

mcli item create --board $PORTFOLIOS_BOARD --name "Platform Modernization" \
  --status "On Track" \
  --number 2400000 \
  --col "$PF_OWNER"="{\"personsAndTeams\":[{\"id\":$ME_ID,\"kind\":\"person\"}]}"
```

The Projects board is the instructive case. RAG (`status`), Portfolio (`text`),
Complete (`numbers`) and Steering Approved (`checkbox`) are each the only column of
their type, so all four have a shorthand. But **Start and Target are both `date`
columns** — `--date` would be ambiguous there, so mcli refuses it and names both
candidates rather than picking one. Those two keep raw `--col`, which is what the
escape hatch exists for:

```sh
mcli item create --board $PROJECTS_BOARD --group $GROUP_PLATFORM --name "Auth service rewrite" \
  --text "Platform Modernization" \
  --status "Green" \
  --number 35 \
  --checkbox true \
  --col "$PROJ_START"='{"date":"2026-01-05"}' \
  --col "$PROJ_TARGET"='{"date":"2026-06-30"}'
```

Where a shorthand does not apply, the raw shapes are: `status` → `{"label":"..."}`,
`date` → `{"date":"YYYY-MM-DD"}`, `people` → `{"personsAndTeams":[...]}`, `checkbox`
→ `{"checked":"true"}`, `numeric` → a quoted string.

Shorthands resolve against the board's shape, so `mcli board column list --board <id>`
is the way to check what is addressable. A board created through the API starts with
only a Name column, so a board you built with `board column create` has exactly the
shape you gave it.

Or using the semantic layer:

```sh
mcli mutation run create_project \
  --var board=$PROJECTS_BOARD \
  --var group=$GROUP_PLATFORM \
  --var name="Auth service rewrite" \
  --var cols='{"portfolio":"Platform Modernization","rag":{"label":"Green"},"complete":"35"}'
```

## Phase 5: Milestones as Subitems

`item create --parent` returns the subitem *and* the board monday put it on, so
the subitems board can be typed on the fly — the first milestone tells you where
the rest of them live:

```sh
MS=$(mcli item create --parent $PROJECT_ID --name "Design sign-off")
MILESTONE_ID=$(jq -r '.id'       <<<"$MS")
SUBITEM_BOARD=$(jq -r '.board.id' <<<"$MS")

mcli board column create --board $SUBITEM_BOARD --title "Due" --type date
mcli board column create --board $SUBITEM_BOARD --title "State" --type status \
  --defaults '{"labels":{"0":"Planned","1":"In Progress","2":"Done","3":"Missed"}}'
```

`item create --parent` returns the created subitem with its board and parent:

```json
{
  "id": "7890123456",
  "name": "Design sign-off",
  "state": "active",
  "board": { "id": "9832181509", "name": "Subitems of Projects" },
  "parent_item": { "id": "7890123400", "name": "Auth service rewrite" }
}
```

Once the columns exist, milestones are updated like any other item — note the
`--board` is the *subitems* board, not the Projects board:

```sh
mcli item update $MILESTONE_ID --board $SUBITEM_BOARD \
  --col "$MS_DUE"='{"date":"2026-02-20"}' \
  --col "$MS_STATE"='{"label":"Done"}'
```

Raw `--col` on purpose here: monday created this board, so its default columns are
not yours to assume. Shorthands suit boards whose shape you defined; on a board you
did not create, address columns by ID (or check with `mcli board column list`).

Later milestones can go through the semantic layer, because the column IDs are
now known:

```sh
mcli mutation run add_milestone \
  --var parent=$PROJECT_ID \
  --var name="Beta in production" \
  --var cols='{"due":{"date":"2026-05-15"},"state":{"label":"Planned"}}'
```

## Phase 6: Rollup Queries

**Projects per portfolio** — group-scoped, so the board does the filtering:

```sh
mcli item list --board $PROJECTS_BOARD --group $GROUP_PLATFORM --limit 100 --pretty
mcli query run projects_in_portfolio --var boardId=$PROJECTS_BOARD --var groupIds="[\"$GROUP_PLATFORM\"]" --pretty
```

The join key works too, when a project may live outside its portfolio's group:

```sh
mcli item find --board $PROJECTS_BOARD --column $PROJ_PORTFOLIO --value "Platform Modernization"
```

**At-risk projects** — status columns match on their label text, so one call per
label answers "what is red?":

```sh
mcli item find --board $PROJECTS_BOARD --column $PROJ_RAG --value "Red"
mcli item find --board $PROJECTS_BOARD --column $PROJ_RAG --value "Amber"
```

Or in a single pass over the board, since `item list` returns decoded column
values (`columns[].value` is the label string for a status column):

```sh
mcli item list --board $PROJECTS_BOARD --limit 100 \
  | jq --arg rag "$PROJ_RAG" '
      .items[]
      | select(any(.columns[]; .id == $rag and (.value == "Red" or .value == "Amber")))
      | {id, name, group: .group.title}'
```

**Portfolio-level rollup** — worst project RAG and mean completion per group:

```sh
mcli item list --board $PROJECTS_BOARD --limit 100 \
  | jq --arg rag "$PROJ_RAG" --arg pct "$PROJ_COMPLETE" '
      def col($id): [.columns[] | select(.id == $id) | .value] | first;
      [.items[] | {portfolio: .group.title, rag: col($rag), pct: col($pct)}]
      | group_by(.portfolio)[]
      | { portfolio: .[0].portfolio,
          projects: length,
          worst_rag: (if any(.[]; .rag == "Red") then "Red"
                      elif any(.[]; .rag == "Amber") then "Amber"
                      else "Green" end),
          avg_complete: (([.[].pct // 0] | add) / length) }'
```

**Milestone status for one project**:

```sh
mcli item get $PROJECT_ID --subitems
mcli item list --board $PROJECTS_BOARD --group $GROUP_PLATFORM --subitems --limit 100 --json
```

`item list` returns the standard list envelope — items with decoded columns plus
a page cursor (`""` means last page):

```json
{
  "items": [
    {
      "id": "7890123400",
      "name": "Auth service rewrite",
      "state": "active",
      "group": { "id": "topics", "title": "Platform Modernization" },
      "columns": [
        { "id": "status", "title": "RAG", "type": "status", "value": "Amber" },
        { "id": "numbers", "title": "Complete", "type": "numbers", "value": 35 }
      ]
    }
  ],
  "cursor": ""
}
```

Escalations belong on the item, where the next reader will find them:

```sh
mcli item post-update $PROJECT_ID --body "RAG → Amber: vendor SSO delivery slipped two weeks."
mcli item description $PROJECT_ID --set "Rewrite of the auth service onto the new platform. Steering: monthly."
```

## Key Takeaways

- **Generic commands** (`board create`, `board group create`, `board column create`, `item create`) handle setup
- **Typed shorthands** address a board's single column of a type (`--status`, `--text`, `--number`, `--checkbox`); two `date` columns on the Projects board is exactly the case where they refuse and `--col` takes over
- **Saved queries/mutations** create a domain-specific portfolio API layer
- **Groups are the rollup axis** — one group per portfolio makes "projects in portfolio" a server-side filter; the `Portfolio` text column plus `item find` covers the cross-group case
- **Subitem boards are discoverable** — `item create --parent` returns `board.id`, which is what you pass to `board column create` and to `item update --board` for milestones
- **Decoded reads** — `item list` / `item get` return status labels and numbers already decoded, so `jq` can roll them up without touching raw monday JSON
- **JSON coercion** — variables declared as `JSON` in the query are auto-stringified, so `--var cols='{"key":"val"}'` just works without double-encoding
- **LLM agents** can operate entirely through `mcli mutation run <name>` / `mcli query run <name>` without understanding monday.com internals or wire-format quirks
- The semantic layer is project-local (`.mcli/` directory) and version-controllable
