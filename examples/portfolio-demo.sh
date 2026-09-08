#!/usr/bin/env bash
# portfolio-demo: set up Portfolios/Projects boards, define a semantic layer of
# saved queries/mutations, seed portfolios and projects, track milestones as
# subitems, then run portfolio rollup queries.
set -euo pipefail

MCLI="${MCLI:-mcli}"
JQ="${JQ:-jq}"

# The authenticated user is used as the owner for every people column.
ME_ID=$($MCLI me | $JQ -r '.id')
PERSON=$(printf '{"personsAndTeams":[{"id":%s,"kind":"person"}]}' "$ME_ID")

# ─── Phase 1: Create Boards ─────────────────────────────────────────────────

echo "=== Phase 1: Create boards ==="

PORTFOLIOS_BOARD=$($MCLI board create --name "Portfolios" --kind public --empty | $JQ -r '.id')
echo "Portfolios board: $PORTFOLIOS_BOARD"

PROJECTS_BOARD=$($MCLI board create --name "Projects" --kind public --empty | $JQ -r '.id')
echo "Projects board: $PROJECTS_BOARD"

# One group per portfolio: "projects in this portfolio" becomes a server-side
# filter (item list --group) instead of a client-side one.
GROUP_PLATFORM=$($MCLI board group create --board "$PROJECTS_BOARD" --name "Platform Modernization" | $JQ -r '.id')
GROUP_CX=$($MCLI board group create --board "$PROJECTS_BOARD" --name "Customer Experience" | $JQ -r '.id')
echo "Projects groups: platform=$GROUP_PLATFORM, cx=$GROUP_CX"

# ─── Phase 2: Add Columns ───────────────────────────────────────────────────

echo ""
echo "=== Phase 2: Define columns ==="

# The typed shorthands used below (--text, --status, --number, --checkbox) address the
# board's single column of that type. A board created through the API starts with only
# a Name column, so each board's shape is exactly what this script gives it. Note
# Projects gets two date columns (Start, Target) on purpose — --date is ambiguous
# there and mcli refuses it, naming both, rather than picking one.

# Portfolios: name (built-in), owner, budget, health
PF_OWNER=$($MCLI board column create --board "$PORTFOLIOS_BOARD" --title "Owner" --type people | $JQ -r '.id')
PF_BUDGET=$($MCLI board column create --board "$PORTFOLIOS_BOARD" --title "Budget" --type numbers | $JQ -r '.id')
PF_HEALTH=$($MCLI board column create --board "$PORTFOLIOS_BOARD" --title "Health" --type status \
  --defaults '{"labels":{"0":"On Track","1":"At Risk","2":"Off Track"}}' | $JQ -r '.id')
echo "Portfolios columns: Owner=$PF_OWNER, Budget=$PF_BUDGET, Health=$PF_HEALTH"

# Projects: Portfolio holds the portfolio name — the join key to Portfolios.
PROJ_PORTFOLIO=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Portfolio" --type text | $JQ -r '.id')
PROJ_OWNER=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Owner" --type people | $JQ -r '.id')
PROJ_START=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Start" --type date | $JQ -r '.id')
PROJ_TARGET=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Target" --type date | $JQ -r '.id')
PROJ_COMPLETE=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Complete" --type numbers | $JQ -r '.id')
PROJ_APPROVED=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Steering Approved" --type checkbox | $JQ -r '.id')
PROJ_NOTES=$($MCLI board column create --board "$PROJECTS_BOARD" --title "Risk Notes" --type long_text | $JQ -r '.id')
PROJ_RAG=$($MCLI board column create --board "$PROJECTS_BOARD" --title "RAG" --type status \
  --defaults '{"labels":{"0":"Green","1":"Amber","2":"Red"}}' | $JQ -r '.id')
echo "Projects columns: Portfolio=$PROJ_PORTFOLIO, Owner=$PROJ_OWNER, Start=$PROJ_START, Target=$PROJ_TARGET"
echo "                  Complete=$PROJ_COMPLETE, Approved=$PROJ_APPROVED, Notes=$PROJ_NOTES, RAG=$PROJ_RAG"

$MCLI board column describe --board "$PROJECTS_BOARD" --column "$PROJ_RAG" \
  --text "Delivery health: Green = on plan, Amber = recoverable slip, Red = escalate" > /dev/null
$MCLI board column describe --board "$PROJECTS_BOARD" --column "$PROJ_PORTFOLIO" \
  --text "Portfolio name — join key to the Portfolios board (item name)" > /dev/null
echo "Described the RAG and join-key columns"

# Note: a timeline column (--type timeline) is also available and mcli decodes it
# as {"from":...,"to":...}; this demo uses two date columns because {"date":...}
# is the documented --col write shape.

# ─── Phase 3: Save Semantic Layer (queries & mutations) ──────────────────────

echo ""
echo "=== Phase 3: Define semantic layer ==="

# --- Queries ---

$MCLI query save list_portfolios --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 100) { items { id name column_values { id text value } } } } }"
echo "Saved query: list_portfolios"

$MCLI query save list_projects --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 100) { items { id name group { id title } column_values { id text value } subitems { id name column_values { id text value } } } } } }"
echo "Saved query: list_projects"

$MCLI query save projects_in_portfolio --query \
  "query(\$boardId: ID!, \$groupIds: [String!]) { boards(ids: [\$boardId]) { groups(ids: \$groupIds) { id title items_page(limit: 100) { items { id name column_values { id text value } subitems { id name } } } } } }"
echo "Saved query: projects_in_portfolio"

$MCLI query save get_project --query \
  "query(\$itemId: [ID!]!) { items(ids: \$itemId) { id name column_values { id text value } subitems { id name column_values { id text value } } } }"
echo "Saved query: get_project"

# --- Mutations ---

$MCLI mutation save create_portfolio --query \
  "mutation(\$board: ID!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_portfolio"

$MCLI mutation save create_project --query \
  "mutation(\$board: ID!, \$group: String!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, group_id: \$group, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_project"

$MCLI mutation save set_project_health --query \
  "mutation(\$board: ID!, \$item: ID!, \$cols: JSON!) { change_multiple_column_values(board_id: \$board, item_id: \$item, column_values: \$cols) { id } }"
echo "Saved mutation: set_project_health"

$MCLI mutation save set_portfolio_health --query \
  "mutation(\$board: ID!, \$item: ID!, \$cols: JSON!) { change_multiple_column_values(board_id: \$board, item_id: \$item, column_values: \$cols) { id } }"
echo "Saved mutation: set_portfolio_health"

$MCLI mutation save add_milestone --query \
  "mutation(\$parent: ID!, \$name: String!, \$cols: JSON!) { create_subitem(parent_item_id: \$parent, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: add_milestone"

# ─── Phase 4: Seed Portfolios & Projects ─────────────────────────────────────

echo ""
echo "=== Phase 4: Seed portfolios and projects ==="

# Health is the board's only status column and Budget its only numbers column, so
# --status and --number address them by type. Owner (people) has no shorthand yet.
PF_PLATFORM=$($MCLI item create --board "$PORTFOLIOS_BOARD" --name "Platform Modernization" \
  --status "On Track" \
  --number 2400000 \
  --col "$PF_OWNER"="$PERSON" | $JQ -r '.id')
echo "Created portfolio: Platform Modernization ($PF_PLATFORM)"

PF_CX=$($MCLI item create --board "$PORTFOLIOS_BOARD" --name "Customer Experience" \
  --status "On Track" \
  --number 900000 \
  --col "$PF_OWNER"="$PERSON" | $JQ -r '.id')
echo "Created portfolio: Customer Experience ($PF_CX)"

# Projects — structured command, one per portfolio group.
#
# This board is the interesting case for shorthands. RAG (status), Portfolio (text),
# Complete (numbers) and Steering Approved (checkbox) are each the board's only
# column of their type, so --status/--text/--number/--checkbox all work. But Start
# and Target are *both* date columns: --date would be ambiguous, so mcli refuses it
# and names both candidates rather than picking one. Those two keep raw --col, which
# is exactly what the escape hatch is for.
PROJ_AUTH=$($MCLI item create --board "$PROJECTS_BOARD" --group "$GROUP_PLATFORM" --name "Auth service rewrite" \
  --text "Platform Modernization" \
  --status "Green" \
  --number 35 \
  --checkbox true \
  --col "$PROJ_START"='{"date":"2026-01-05"}' \
  --col "$PROJ_TARGET"='{"date":"2026-06-30"}' \
  --col "$PROJ_OWNER"="$PERSON" | $JQ -r '.id')
echo "Created project: Auth service rewrite ($PROJ_AUTH)"

PROJ_DATA=$($MCLI item create --board "$PROJECTS_BOARD" --group "$GROUP_PLATFORM" --name "Data lake migration" \
  --text "Platform Modernization" \
  --status "Amber" \
  --number 20 \
  --checkbox true \
  --col "$PROJ_START"='{"date":"2026-02-01"}' \
  --col "$PROJ_TARGET"='{"date":"2026-09-30"}' \
  --col "$PROJ_OWNER"="$PERSON" | $JQ -r '.id')
echo "Created project: Data lake migration ($PROJ_DATA)"

# Third project via the semantic layer — same result, business-level call.
PROJECT_COLS=$(printf '{ "%s": "%s", "%s": {"label": "Green"}, "%s": {"date": "%s"}, "%s": {"date": "%s"}, "%s": "%s", "%s": %s }' \
  "$PROJ_PORTFOLIO" "Customer Experience" "$PROJ_RAG" \
  "$PROJ_START" "2026-03-02" "$PROJ_TARGET" "2026-08-14" \
  "$PROJ_COMPLETE" "10" "$PROJ_OWNER" "$PERSON")
PROJ_PORTAL=$($MCLI mutation run create_project \
  --var board="$PROJECTS_BOARD" \
  --var group="$GROUP_CX" \
  --var name="Self-serve portal" \
  --var cols="$PROJECT_COLS" | $JQ -r '.data.create_item.id')
echo "Created project: Self-serve portal ($PROJ_PORTAL)"

# ─── Phase 5: Milestones as Subitems ─────────────────────────────────────────

echo ""
echo "=== Phase 5: Milestones ==="

# The first subitem tells us which board monday put milestones on, so we can
# give that board real columns.
MS_FIRST=$($MCLI item create --parent "$PROJ_AUTH" --name "Design sign-off")
MS_DESIGN=$($JQ -r '.id' <<<"$MS_FIRST")
SUBITEM_BOARD=$($JQ -r '.board.id' <<<"$MS_FIRST")
echo "Milestone subitem board: $SUBITEM_BOARD"

MS_DUE=$($MCLI board column create --board "$SUBITEM_BOARD" --title "Due" --type date | $JQ -r '.id')
MS_STATE=$($MCLI board column create --board "$SUBITEM_BOARD" --title "State" --type status \
  --defaults '{"labels":{"0":"Planned","1":"In Progress","2":"Done","3":"Missed"}}' | $JQ -r '.id')
echo "Milestone columns: Due=$MS_DUE, State=$MS_STATE"

# Backfill the first milestone now that the columns exist. Note the --board is
# the subitems board, not the Projects board.
#
# Raw --col here on purpose: monday creates the subitems board itself, so its
# default columns are not ours to assume. Shorthands are for boards whose shape you
# defined — on a board you did not create, address columns by ID.
$MCLI item update "$MS_DESIGN" --board "$SUBITEM_BOARD" \
  --col "$MS_DUE"='{"date":"2026-02-20"}' \
  --col "$MS_STATE"='{"label":"Done"}' > /dev/null
echo "  Design sign-off — due 2026-02-20 — Done"

# Remaining milestones go through the semantic layer.
MILESTONE_COLS=$(printf '{ "%s": {"date": "%s"}, "%s": {"label": "%s"} }' "$MS_DUE" "2026-05-15" "$MS_STATE" "In Progress")
$MCLI mutation run add_milestone \
  --var parent="$PROJ_AUTH" \
  --var name="Beta in production" \
  --var cols="$MILESTONE_COLS" | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Beta in production — due 2026-05-15 — In Progress"

MILESTONE_COLS=$(printf '{ "%s": {"date": "%s"}, "%s": {"label": "%s"} }' "$MS_DUE" "2026-06-30" "$MS_STATE" "Planned")
$MCLI mutation run add_milestone \
  --var parent="$PROJ_AUTH" \
  --var name="Legacy service decommissioned" \
  --var cols="$MILESTONE_COLS" | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Legacy service decommissioned — due 2026-06-30 — Planned"

MILESTONE_COLS=$(printf '{ "%s": {"date": "%s"}, "%s": {"label": "%s"} }' "$MS_DUE" "2026-04-10" "$MS_STATE" "Missed")
$MCLI mutation run add_milestone \
  --var parent="$PROJ_DATA" \
  --var name="Schema freeze" \
  --var cols="$MILESTONE_COLS" | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Schema freeze (Data lake migration) — due 2026-04-10 — Missed"

# A missed milestone drives the project — and then the portfolio — off Green.
HEALTH_COLS=$(printf '{ "%s": {"label": "Red"}, "%s": {"text": "%s"} }' \
  "$PROJ_RAG" "$PROJ_NOTES" "Schema freeze missed; downstream ETL rework needed. Escalated to steering.")
$MCLI mutation run set_project_health \
  --var board="$PROJECTS_BOARD" \
  --var item="$PROJ_DATA" \
  --var cols="$HEALTH_COLS" | $JQ -r '.data' > /dev/null
echo "Data lake migration RAG → Red"

$MCLI item post-update "$PROJ_DATA" \
  --body "RAG → Red: schema freeze slipped, ETL rework in scope. Recovery plan at next steering." > /dev/null
$MCLI item description "$PROJ_DATA" \
  --set "Migration of the analytics estate onto the new data lake. Steering: monthly." > /dev/null
echo "Escalation update + project brief posted"

PF_COLS=$(printf '{ "%s": {"label": "At Risk"} }' "$PF_HEALTH")
$MCLI mutation run set_portfolio_health \
  --var board="$PORTFOLIOS_BOARD" \
  --var item="$PF_PLATFORM" \
  --var cols="$PF_COLS" | $JQ -r '.data' > /dev/null
echo "Platform Modernization portfolio health → At Risk"

# ─── Phase 6: Rollup Queries ─────────────────────────────────────────────────

echo ""
echo "=== Phase 6: Rollups ==="

echo "--- Projects in Platform Modernization (group-scoped) ---"
$MCLI item list --board "$PROJECTS_BOARD" --group "$GROUP_PLATFORM" --limit 100 --pretty

echo ""
echo "--- Same, via the semantic layer ---"
$MCLI query run projects_in_portfolio \
  --var boardId="$PROJECTS_BOARD" \
  --var groupIds="$(printf '["%s"]' "$GROUP_PLATFORM")" --pretty

echo ""
echo "--- Projects joined by portfolio name ---"
$MCLI item find --board "$PROJECTS_BOARD" --column "$PROJ_PORTFOLIO" --value "Platform Modernization" --pretty

echo ""
echo "--- At-risk projects (status columns match on label text) ---"
$MCLI item find --board "$PROJECTS_BOARD" --column "$PROJ_RAG" --value "Red" --pretty
$MCLI item find --board "$PROJECTS_BOARD" --column "$PROJ_RAG" --value "Amber" --pretty

echo ""
echo "--- At-risk projects in one pass (decoded column values) ---"
$MCLI item list --board "$PROJECTS_BOARD" --limit 100 \
  | $JQ --arg rag "$PROJ_RAG" '
      .items[]
      | select(any(.columns[]; .id == $rag and (.value == "Red" or .value == "Amber")))
      | {id, name, portfolio: .group.title}'

echo ""
echo "--- Portfolio rollup: worst RAG and mean completion per portfolio group ---"
$MCLI item list --board "$PROJECTS_BOARD" --limit 100 \
  | $JQ --arg rag "$PROJ_RAG" --arg pct "$PROJ_COMPLETE" '
      def col($id): [.columns[] | select(.id == $id) | .value] | first;
      [.items[] | {portfolio: .group.title, rag: col($rag), pct: col($pct)}]
      | group_by(.portfolio)[]
      | { portfolio: .[0].portfolio,
          projects: length,
          worst_rag: (if any(.[]; .rag == "Red") then "Red"
                      elif any(.[]; .rag == "Amber") then "Amber"
                      else "Green" end),
          avg_complete: (([.[].pct // 0] | add) / length) }'

echo ""
echo "--- Milestones for Auth service rewrite ---"
$MCLI item get "$PROJ_AUTH" --subitems --pretty

echo ""
echo "--- Every project with its milestones (--subitems is JSON-only) ---"
$MCLI item list --board "$PROJECTS_BOARD" --subitems --limit 100 --json | $JQ '.'

echo ""
echo "=== Demo complete ==="
echo ""
echo "Board IDs for further exploration:"
echo "  Portfolios: $PORTFOLIOS_BOARD"
echo "  Projects:   $PROJECTS_BOARD (groups: platform=$GROUP_PLATFORM, cx=$GROUP_CX)"
echo "  Milestones: $SUBITEM_BOARD (subitems of Projects)"
