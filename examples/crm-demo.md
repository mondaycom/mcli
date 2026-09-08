# CRM Demo: Companies, Contacts & Deals

This demo shows how an LLM agent (or human) can use mcli's generic commands to:

1. **Create boards** with typed columns and pipeline groups
2. **Define a semantic layer** of saved queries/mutations with business-domain names
3. **Seed accounts and contacts** using the structured item commands
4. **Run a deal through the pipeline** using only the semantic layer (no board IDs in the "business logic")
5. **Query the pipeline** — by account, by stage, with activity subitems

## Prerequisites

- `mcli auth login` (or `MONDAY_API_TOKEN` set)
- `jq` in PATH

Run the full demo: `./examples/crm-demo.sh`

---

## Phase 1: Create Boards

```sh
mcli board create --name "Companies" --kind public --empty   # → {"id":"..."}
mcli board create --name "Contacts"  --kind public --empty
mcli board create --name "Deals"     --kind public --empty
```

Each board starts empty (no default columns/items).

The Deals board also gets two groups so the pipeline has a physical shape —
open deals live in one group, closed deals in another:

```sh
mcli board group create --board $DEALS_BOARD --name "Open Pipeline"
mcli board group create --board $DEALS_BOARD --name "Closed"
```

## Phase 2: Define Columns

**Companies** — item name is the company name:

```sh
mcli board column create --board $COMPANIES_BOARD --title "Domain"  --type text
mcli board column create --board $COMPANIES_BOARD --title "Website" --type link
mcli board column create --board $COMPANIES_BOARD --title "Owner"   --type people
mcli board column create --board $COMPANIES_BOARD --title "Segment" --type status \
  --defaults '{"labels":{"0":"SMB","1":"Mid-Market","2":"Enterprise"}}'
```

**Contacts** — item name is the person's name; `Company` holds the account's
domain, which is the join key back to the Companies board:

```sh
mcli board column create --board $CONTACTS_BOARD --title "Company" --type text
mcli board column create --board $CONTACTS_BOARD --title "Title"   --type text
mcli board column create --board $CONTACTS_BOARD --title "Email"   --type text
mcli board column create --board $CONTACTS_BOARD --title "Owner"   --type people
```

**Deals** — item name is the deal name; subitems are pipeline activities:

```sh
mcli board column create --board $DEALS_BOARD --title "Company"        --type text
mcli board column create --board $DEALS_BOARD --title "Value"          --type numbers
mcli board column create --board $DEALS_BOARD --title "Expected Close" --type date
mcli board column create --board $DEALS_BOARD --title "Owner"          --type people
mcli board column create --board $DEALS_BOARD --title "Stage"          --type status \
  --defaults '{"labels":{"0":"Discovery","1":"Qualified","2":"Proposal","3":"Negotiation","4":"Won","5":"Lost"}}'
```

Column descriptions make the board self-documenting for the next agent:

```sh
mcli board column describe --board $DEALS_BOARD --column $DEAL_COMPANY \
  --text "Account domain — join key to the Companies board (Domain column)"
```

### A note on linking boards

mcli can create a `board_relation` column (`--type board_relation`), but the
link configuration (which boards are connected) is not modelled by the CLI, so
this demo uses an explicit join key instead: every Contact and Deal stores the
account **domain** in a text column, and `mcli item find` resolves it. Contacts
and Deals therefore stay independent boards, while the *activity* relationship —
which monday models natively — uses **subitems** under each deal.

## Phase 3: Semantic Layer

Saved queries and mutations give business-level names to operations. An LLM can
call `mcli mutation run advance_deal_stage --var ...` without knowing GraphQL:

### Queries

| Name | Purpose |
|------|---------|
| `list_companies` | All accounts with column values |
| `list_contacts` | All contacts with column values |
| `list_pipeline` | Deals with their activity subitems |
| `get_deal` | Single deal detail by item ID |

```sh
mcli query save list_pipeline --query \
  'query($boardId: ID!) { boards(ids: [$boardId]) { items_page(limit:100) { items { id name column_values { id text value } subitems { id name } } } } }'
```

### Mutations

| Name | Purpose |
|------|---------|
| `create_company` | Add an account |
| `create_contact` | Add a contact against an account |
| `create_deal` | Open a new deal (starts in Discovery) |
| `advance_deal_stage` | Move a deal to the next stage |
| `log_deal_activity` | Add an activity subitem to a deal |

```sh
mcli mutation save advance_deal_stage --query \
  'mutation($board: ID!, $item: ID!, $cols: JSON!) { change_multiple_column_values(board_id: $board, item_id: $item, column_values: $cols) { id } }'
```

## Phase 4: Seed Accounts & Contacts

Using the structured `item create` command (more ergonomic for setup). Domain is the
Companies board's only `text` column and Segment its only `status` column, so `--text`
and `--status` address them by type — no column ID, and `"Enterprise"` is checked
against the board's own labels before anything is sent. Website (`link`) and Owner
(`people`) have no shorthand, so they use monday's raw write shapes via `--col`:

```sh
ME_ID=$(mcli me | jq -r '.id')

mcli item create --board $COMPANIES_BOARD --name "Acme Corp" \
  --text "acme.com" \
  --status "Enterprise" \
  --col "$CO_WEBSITE"='{"url":"https://acme.com","text":"acme.com"}' \
  --col "$CO_OWNER"="{\"personsAndTeams\":[{\"id\":$ME_ID,\"kind\":\"person\"}]}"
```

Shorthands and `--col` mix freely on one command. Where a shorthand does not apply,
the raw shapes are: `status` → `{"label":"..."}`, `people` →
`{"personsAndTeams":[...]}`, `numeric` → a quoted string.

The Contacts board is the counter-example: it has three `text` columns (Company,
Title, Email), so `--text` would be ambiguous and mcli refuses it, naming all three
rather than guessing. Address those by ID — or by domain name through the semantic
layer, which is what it is for.

Or using the semantic layer:

```sh
mcli mutation run create_company \
  --var board=$COMPANIES_BOARD \
  --var name="Acme Corp" \
  --var cols='{"domain":"acme.com","segment":{"label":"Enterprise"}}'
```

## Phase 5: Run a Deal Through the Pipeline

This is where the semantic layer shines — the agent writes natural JSON in `--var`
and mcli handles the encoding automatically. monday.com's `JSON` scalar expects a
stringified JSON value on the wire, but mcli detects variables declared as `JSON`
in the query and re-encodes them transparently. No double-escaping needed.

```sh
# 1. Open the deal in Discovery, in the "Open Pipeline" group
mcli mutation run create_deal \
  --var board=$DEALS_BOARD \
  --var group=$GROUP_OPEN \
  --var name="Acme — Platform rollout" \
  --var cols='{"company":"acme.com","stage":{"label":"Discovery"},"value":"120000","expected_close":{"date":"2026-12-15"}}'

# 2. Log activities as subitems
mcli mutation run log_deal_activity --var parent=$DEAL_ID --var name="Discovery call with VP Eng" --var cols='{}'
mcli mutation run log_deal_activity --var parent=$DEAL_ID --var name="Sent proposal v1"          --var cols='{}'

# 3. Advance the stage (repeat per transition)
mcli mutation run advance_deal_stage \
  --var board=$DEALS_BOARD --var item=$DEAL_ID \
  --var cols='{"stage":{"label":"Qualified"}}'
```

The same transition through the structured command, when you prefer flags over
GraphQL — and a raised deal value on the way to Proposal. The Deals board has exactly
one `status` and one `numbers` column, so the whole update reads in business terms
with no column IDs and no wire JSON:

```sh
mcli item update $DEAL_ID --board $DEALS_BOARD \
  --status "Proposal" \
  --number 145000
```

Narrative belongs on the item itself, not in a column:

```sh
mcli item description $DEAL_ID --set "Multi-region rollout. Champion: VP Eng. Blocker: security review."
mcli item post-update  $DEAL_ID --body "Stage → Negotiation. Legal reviewing MSA redlines."
```

When the deal closes, mark it Won and move it out of the open pipeline:

```sh
mcli item update $DEAL_ID --board $DEALS_BOARD --status "Won"
mcli item move   $DEAL_ID --to-group $GROUP_CLOSED
```

## Phase 6: Query the Pipeline

```sh
# Everything on the account, resolved through the domain join key
mcli item find --board $CONTACTS_BOARD --column $CT_COMPANY --value "acme.com"
mcli item find --board $DEALS_BOARD    --column $DEAL_COMPANY --value "acme.com"

# Deals in one stage — status columns match on their label text
mcli item find --board $DEALS_BOARD --column $DEAL_STAGE --value "Won"

# Open pipeline only (group-scoped), with activity subitems (--subitems is JSON-only)
mcli item list --board $DEALS_BOARD --group $GROUP_OPEN --subitems --limit 100 --json

# Full deal detail, subitem column values included
mcli item get $DEAL_ID --subitems

# Semantic layer read path
mcli query run list_pipeline --var boardId=$DEALS_BOARD --pretty
mcli query run get_deal --var itemId="[$DEAL_ID]" --pretty
```

`item find` returns the id/name/group triple plus a page cursor:

```json
{
  "items": [
    { "id": "7890123456", "name": "Acme — Platform rollout", "group": { "id": "topics", "title": "Closed" } }
  ],
  "cursor": ""
}
```

Total open pipeline value, straight out of `item list` (column values arrive
already decoded, so `jq` can sum them):

```sh
mcli item list --board $DEALS_BOARD --group $GROUP_OPEN --limit 100 \
  | jq --arg col "$DEAL_VALUE" '[.items[].columns[] | select(.id == $col) | .value | tonumber] | add'
```

## Key Takeaways

- **Generic commands** (`board create`, `board group create`, `board column create`, `item create`) handle setup
- **Typed shorthands** (`--status`, `--text`, `--number`) drop column IDs and wire JSON where the board has one column of that type; validated before send, and they refuse rather than guess when it has several
- **Saved queries/mutations** create a domain-specific CRM API layer
- **A text join key** (the account domain) plus `mcli item find` links boards without needing `board_relation` link settings
- **Subitems** model the natural one-to-many relationship (deal → activities)
- **Groups** model pipeline state you want to *see*; the status column models the stage you want to *query*
- **JSON coercion** — variables declared as `JSON` in the query are auto-stringified, so `--var cols='{"key":"val"}'` just works without double-encoding
- **LLM agents** can operate entirely through `mcli mutation run <name>` / `mcli query run <name>` without understanding monday.com internals or wire-format quirks
- The semantic layer is project-local (`.mcli/` directory) and version-controllable
