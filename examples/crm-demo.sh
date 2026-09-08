#!/usr/bin/env bash
# crm-demo: set up Companies/Contacts/Deals boards, define a semantic layer of
# saved queries/mutations, seed accounts and contacts, then run a deal through
# the pipeline and query it.
set -euo pipefail

MCLI="${MCLI:-mcli}"
JQ="${JQ:-jq}"

# The authenticated user is used as the owner for every people column.
ME_ID=$($MCLI me | $JQ -r '.id')
PERSON=$(printf '{"personsAndTeams":[{"id":%s,"kind":"person"}]}' "$ME_ID")

# ─── Phase 1: Create Boards ─────────────────────────────────────────────────

echo "=== Phase 1: Create boards ==="

COMPANIES_BOARD=$($MCLI board create --name "Companies" --kind public --empty | $JQ -r '.id')
echo "Companies board: $COMPANIES_BOARD"

CONTACTS_BOARD=$($MCLI board create --name "Contacts" --kind public --empty | $JQ -r '.id')
echo "Contacts board: $CONTACTS_BOARD"

DEALS_BOARD=$($MCLI board create --name "Deals" --kind public --empty | $JQ -r '.id')
echo "Deals board: $DEALS_BOARD"

# Pipeline groups: open deals stay in one group, closed deals move to the other.
GROUP_OPEN=$($MCLI board group create --board "$DEALS_BOARD" --name "Open Pipeline" | $JQ -r '.id')
GROUP_CLOSED=$($MCLI board group create --board "$DEALS_BOARD" --name "Closed" | $JQ -r '.id')
echo "Deals groups: open=$GROUP_OPEN, closed=$GROUP_CLOSED"

# ─── Phase 2: Add Columns ───────────────────────────────────────────────────

echo ""
echo "=== Phase 2: Define columns ==="

# The typed shorthands used below (--text, --status, --number) address the board's
# single column of that type. A board created through the API starts with only a Name
# column, so each board's shape is exactly what this script gives it. Note Contacts
# gets three text columns on purpose — --text is ambiguous there and mcli refuses it,
# naming the candidates, rather than picking one.

# Companies: name (built-in), domain, website, owner, segment
CO_DOMAIN=$($MCLI board column create --board "$COMPANIES_BOARD" --title "Domain" --type text | $JQ -r '.id')
CO_WEBSITE=$($MCLI board column create --board "$COMPANIES_BOARD" --title "Website" --type link | $JQ -r '.id')
CO_OWNER=$($MCLI board column create --board "$COMPANIES_BOARD" --title "Owner" --type people | $JQ -r '.id')
CO_SEGMENT=$($MCLI board column create --board "$COMPANIES_BOARD" --title "Segment" --type status \
  --defaults '{"labels":{"0":"SMB","1":"Mid-Market","2":"Enterprise"}}' | $JQ -r '.id')
echo "Companies columns: Domain=$CO_DOMAIN, Website=$CO_WEBSITE, Owner=$CO_OWNER, Segment=$CO_SEGMENT"

# Contacts: Company holds the account domain — the join key to Companies.
CT_COMPANY=$($MCLI board column create --board "$CONTACTS_BOARD" --title "Company" --type text | $JQ -r '.id')
CT_TITLE=$($MCLI board column create --board "$CONTACTS_BOARD" --title "Title" --type text | $JQ -r '.id')
CT_EMAIL=$($MCLI board column create --board "$CONTACTS_BOARD" --title "Email" --type text | $JQ -r '.id')
CT_OWNER=$($MCLI board column create --board "$CONTACTS_BOARD" --title "Owner" --type people | $JQ -r '.id')
echo "Contacts columns: Company=$CT_COMPANY, Title=$CT_TITLE, Email=$CT_EMAIL, Owner=$CT_OWNER"

# Deals: company (join key), value, expected close, owner, stage
DEAL_COMPANY=$($MCLI board column create --board "$DEALS_BOARD" --title "Company" --type text | $JQ -r '.id')
DEAL_VALUE=$($MCLI board column create --board "$DEALS_BOARD" --title "Value" --type numbers | $JQ -r '.id')
DEAL_CLOSE=$($MCLI board column create --board "$DEALS_BOARD" --title "Expected Close" --type date | $JQ -r '.id')
DEAL_OWNER=$($MCLI board column create --board "$DEALS_BOARD" --title "Owner" --type people | $JQ -r '.id')
DEAL_STAGE=$($MCLI board column create --board "$DEALS_BOARD" --title "Stage" --type status \
  --defaults '{"labels":{"0":"Discovery","1":"Qualified","2":"Proposal","3":"Negotiation","4":"Won","5":"Lost"}}' | $JQ -r '.id')
echo "Deals columns: Company=$DEAL_COMPANY, Value=$DEAL_VALUE, Close=$DEAL_CLOSE, Owner=$DEAL_OWNER, Stage=$DEAL_STAGE"

# Self-documenting boards: describe the non-obvious column.
$MCLI board column describe --board "$DEALS_BOARD" --column "$DEAL_COMPANY" \
  --text "Account domain — join key to the Companies board (Domain column)" > /dev/null
$MCLI board column describe --board "$CONTACTS_BOARD" --column "$CT_COMPANY" \
  --text "Account domain — join key to the Companies board (Domain column)" > /dev/null
echo "Described the join-key columns"

# Note: mcli can create a board_relation column (--type board_relation), but the
# board-link settings are not modelled by the CLI, so this demo joins boards on
# the account domain via 'mcli item find'. Deal activities use native subitems.

# ─── Phase 3: Save Semantic Layer (queries & mutations) ──────────────────────

echo ""
echo "=== Phase 3: Define semantic layer ==="

# --- Queries ---

$MCLI query save list_companies --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 100) { items { id name column_values { id text value } } } } }"
echo "Saved query: list_companies"

$MCLI query save list_contacts --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 200) { items { id name column_values { id text value } } } } }"
echo "Saved query: list_contacts"

$MCLI query save list_pipeline --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 100) { items { id name group { id title } column_values { id text value } subitems { id name } } } } }"
echo "Saved query: list_pipeline"

$MCLI query save get_deal --query \
  "query(\$itemId: [ID!]!) { items(ids: \$itemId) { id name column_values { id text value } subitems { id name column_values { id text value } } } }"
echo "Saved query: get_deal"

# --- Mutations ---

$MCLI mutation save create_company --query \
  "mutation(\$board: ID!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_company"

$MCLI mutation save create_contact --query \
  "mutation(\$board: ID!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_contact"

$MCLI mutation save create_deal --query \
  "mutation(\$board: ID!, \$group: String!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, group_id: \$group, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_deal"

$MCLI mutation save advance_deal_stage --query \
  "mutation(\$board: ID!, \$item: ID!, \$cols: JSON!) { change_multiple_column_values(board_id: \$board, item_id: \$item, column_values: \$cols) { id } }"
echo "Saved mutation: advance_deal_stage"

$MCLI mutation save log_deal_activity --query \
  "mutation(\$parent: ID!, \$name: String!, \$cols: JSON!) { create_subitem(parent_item_id: \$parent, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: log_deal_activity"

# ─── Phase 4: Seed Accounts & Contacts ───────────────────────────────────────

echo ""
echo "=== Phase 4: Seed accounts and contacts ==="

# Companies (structured command — most ergonomic for setup).
#
# Domain is the board's only text column and Segment its only status column, so
# --text and --status address them by type — no column ID, and the label is checked
# against the board's own labels before anything is sent. Website (link) and Owner
# (people) have no shorthand, so they stay on raw --col: the two forms mix freely.
CO_ACME=$($MCLI item create --board "$COMPANIES_BOARD" --name "Acme Corp" \
  --text "acme.com" \
  --status "Enterprise" \
  --col "$CO_WEBSITE"='{"url":"https://acme.com","text":"acme.com"}' \
  --col "$CO_OWNER"="$PERSON" | $JQ -r '.id')
echo "Created company: Acme Corp ($CO_ACME)"

CO_GLOBEX=$($MCLI item create --board "$COMPANIES_BOARD" --name "Globex" \
  --text "globex.io" \
  --status "Mid-Market" \
  --col "$CO_WEBSITE"='{"url":"https://globex.io","text":"globex.io"}' \
  --col "$CO_OWNER"="$PERSON" | $JQ -r '.id')
echo "Created company: Globex ($CO_GLOBEX)"

# Contacts (semantic layer — same result, business-level call).
#
# Note this board has three text columns (Company, Title, Email), so --text would
# be ambiguous here and mcli would refuse rather than guess. Columns are addressed
# by ID, which is what the semantic layer is for.
CONTACT_COLS=$(printf '{ "%s": "%s", "%s": "%s", "%s": "%s", "%s": %s }' \
  "$CT_COMPANY" "acme.com" "$CT_TITLE" "VP Engineering" "$CT_EMAIL" "dana@acme.com" "$CT_OWNER" "$PERSON")
CT_DANA=$($MCLI mutation run create_contact \
  --var board="$CONTACTS_BOARD" \
  --var name="Dana Reyes" \
  --var cols="$CONTACT_COLS" | $JQ -r '.data.create_item.id')
echo "Created contact: Dana Reyes ($CT_DANA) @ acme.com"

CONTACT_COLS=$(printf '{ "%s": "%s", "%s": "%s", "%s": "%s", "%s": %s }' \
  "$CT_COMPANY" "acme.com" "$CT_TITLE" "Head of Procurement" "$CT_EMAIL" "sam@acme.com" "$CT_OWNER" "$PERSON")
CT_SAM=$($MCLI mutation run create_contact \
  --var board="$CONTACTS_BOARD" \
  --var name="Sam Okafor" \
  --var cols="$CONTACT_COLS" | $JQ -r '.data.create_item.id')
echo "Created contact: Sam Okafor ($CT_SAM) @ acme.com"

CONTACT_COLS=$(printf '{ "%s": "%s", "%s": "%s", "%s": "%s", "%s": %s }' \
  "$CT_COMPANY" "globex.io" "$CT_TITLE" "CTO" "$CT_EMAIL" "lee@globex.io" "$CT_OWNER" "$PERSON")
CT_LEE=$($MCLI mutation run create_contact \
  --var board="$CONTACTS_BOARD" \
  --var name="Lee Tran" \
  --var cols="$CONTACT_COLS" | $JQ -r '.data.create_item.id')
echo "Created contact: Lee Tran ($CT_LEE) @ globex.io"

# ─── Phase 5: Run a Deal Through the Pipeline ────────────────────────────────

echo ""
echo "=== Phase 5: Open a deal and work the pipeline ==="

# Open the deal in Discovery, inside the "Open Pipeline" group.
DEAL_COLS=$(printf '{ "%s": "%s", "%s": {"label": "Discovery"}, "%s": "%s", "%s": {"date": "%s"}, "%s": %s }' \
  "$DEAL_COMPANY" "acme.com" "$DEAL_STAGE" "$DEAL_VALUE" "120000" "$DEAL_CLOSE" "2026-12-15" "$DEAL_OWNER" "$PERSON")
DEAL_ID=$($MCLI mutation run create_deal \
  --var board="$DEALS_BOARD" \
  --var group="$GROUP_OPEN" \
  --var name="Acme — Platform rollout" \
  --var cols="$DEAL_COLS" | $JQ -r '.data.create_item.id')
echo "Opened deal: Acme — Platform rollout ($DEAL_ID) — stage: Discovery, \$120,000"

# A second deal, so the pipeline has more than one row to roll up.
DEAL_COLS=$(printf '{ "%s": "%s", "%s": {"label": "Qualified"}, "%s": "%s", "%s": {"date": "%s"}, "%s": %s }' \
  "$DEAL_COMPANY" "globex.io" "$DEAL_STAGE" "$DEAL_VALUE" "38000" "$DEAL_CLOSE" "2026-10-01" "$DEAL_OWNER" "$PERSON")
DEAL_GLOBEX=$($MCLI mutation run create_deal \
  --var board="$DEALS_BOARD" \
  --var group="$GROUP_OPEN" \
  --var name="Globex — Analytics add-on" \
  --var cols="$DEAL_COLS" | $JQ -r '.data.create_item.id')
echo "Opened deal: Globex — Analytics add-on ($DEAL_GLOBEX) — stage: Qualified, \$38,000"

# Log pipeline activities as subitems of the deal.
$MCLI mutation run log_deal_activity \
  --var parent="$DEAL_ID" \
  --var name="Discovery call with VP Engineering" \
  --var cols='{}' | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Activity: discovery call logged"

$MCLI mutation run log_deal_activity \
  --var parent="$DEAL_ID" \
  --var name="Technical deep-dive with platform team" \
  --var cols='{}' | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Activity: technical deep-dive logged"

# Stage transition through the semantic layer.
STAGE_COLS=$(printf '{ "%s": {"label": "Qualified"} }' "$DEAL_STAGE")
$MCLI mutation run advance_deal_stage \
  --var board="$DEALS_BOARD" \
  --var item="$DEAL_ID" \
  --var cols="$STAGE_COLS" | $JQ -r '.data' > /dev/null
echo "Deal stage → Qualified"

# Stage transition through the structured command, raising the value at the same
# time. The Deals board has exactly one status and one numbers column, so the whole
# update reads in business terms with no column IDs and no wire JSON at all.
$MCLI item update "$DEAL_ID" --board "$DEALS_BOARD" \
  --status "Proposal" \
  --number 145000 > /dev/null
echo "Deal stage → Proposal (value raised to \$145,000)"

# Narrative lives on the item, not in a column.
$MCLI item description "$DEAL_ID" \
  --set "Multi-region platform rollout. Champion: VP Engineering. Blocker: security review." > /dev/null
$MCLI item post-update "$DEAL_ID" \
  --body "Stage → Negotiation. Legal is reviewing MSA redlines; close date still 2026-12-15." > /dev/null
echo "Deal brief + update posted"

STAGE_COLS=$(printf '{ "%s": {"label": "Negotiation"} }' "$DEAL_STAGE")
$MCLI mutation run advance_deal_stage \
  --var board="$DEALS_BOARD" \
  --var item="$DEAL_ID" \
  --var cols="$STAGE_COLS" | $JQ -r '.data' > /dev/null
echo "Deal stage → Negotiation"

# Close it: mark Won, then move it out of the open pipeline group.
$MCLI item update "$DEAL_ID" --board "$DEALS_BOARD" --status "Won" > /dev/null
$MCLI item move "$DEAL_ID" --to-group "$GROUP_CLOSED" > /dev/null
echo "Deal stage → Won, moved to the Closed group"

# ─── Phase 6: Query the Pipeline ─────────────────────────────────────────────

echo ""
echo "=== Phase 6: Query the pipeline ==="

echo "--- Contacts at acme.com (join on the domain key) ---"
$MCLI item find --board "$CONTACTS_BOARD" --column "$CT_COMPANY" --value "acme.com" --pretty

echo ""
echo "--- Deals at acme.com ---"
$MCLI item find --board "$DEALS_BOARD" --column "$DEAL_COMPANY" --value "acme.com" --pretty

echo ""
echo "--- Won deals (status columns match on label text) ---"
$MCLI item find --board "$DEALS_BOARD" --column "$DEAL_STAGE" --value "Won" --pretty

echo ""
echo "--- Open pipeline, with activity subitems (--subitems is JSON-only) ---"
$MCLI item list --board "$DEALS_BOARD" --group "$GROUP_OPEN" --subitems --limit 100 --json | $JQ '.'

echo ""
echo "--- Open pipeline value ---"
OPEN_VALUE=$($MCLI item list --board "$DEALS_BOARD" --group "$GROUP_OPEN" --limit 100 \
  | $JQ --arg col "$DEAL_VALUE" '[.items[].columns[] | select(.id == $col) | .value | tonumber] | add // 0')
echo "Open pipeline total: \$$OPEN_VALUE"

echo ""
echo "--- Won deal detail (semantic layer) ---"
$MCLI query run get_deal --var itemId="[$DEAL_ID]" --pretty

echo ""
echo "=== Demo complete ==="
echo ""
echo "Board IDs for further exploration:"
echo "  Companies: $COMPANIES_BOARD"
echo "  Contacts:  $CONTACTS_BOARD"
echo "  Deals:     $DEALS_BOARD (groups: open=$GROUP_OPEN, closed=$GROUP_CLOSED)"
