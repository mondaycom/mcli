#!/usr/bin/env bash
# ecommerce-demo: set up Products/Inventory/Orders boards, define a semantic
# layer of saved queries/mutations, seed data, and place an order.
set -euo pipefail

MCLI="${MCLI:-mcli}"
JQ="${JQ:-jq}"

# ─── Phase 1: Create Boards ─────────────────────────────────────────────────

echo "=== Phase 1: Create boards ==="

PRODUCTS_BOARD=$($MCLI board create --name "Products" --kind public --empty | $JQ -r '.id')
echo "Products board: $PRODUCTS_BOARD"

INVENTORY_BOARD=$($MCLI board create --name "Inventory" --kind public --empty | $JQ -r '.id')
echo "Inventory board: $INVENTORY_BOARD"

ORDERS_BOARD=$($MCLI board create --name "Orders" --kind public --empty | $JQ -r '.id')
echo "Orders board: $ORDERS_BOARD"

# ─── Phase 2: Add Columns ───────────────────────────────────────────────────

echo ""
echo "=== Phase 2: Define columns ==="

# The typed shorthands used below (--text, --number) address the board's single column
# of that type. A board created through the API starts with only a Name column, so each
# board's shape is exactly what this script gives it. If a type is ever duplicated the
# shorthand refuses, naming the candidates, rather than picking one.

# Products: name (built-in), SKU, description, price
PROD_SKU=$($MCLI board column create --board "$PRODUCTS_BOARD" --title "SKU" --type text | $JQ -r '.id')
PROD_DESC=$($MCLI board column create --board "$PRODUCTS_BOARD" --title "Description" --type long_text | $JQ -r '.id')
PROD_PRICE=$($MCLI board column create --board "$PRODUCTS_BOARD" --title "Price" --type numbers | $JQ -r '.id')
echo "Products columns: SKU=$PROD_SKU, Description=$PROD_DESC, Price=$PROD_PRICE"

# Inventory: SKU, quantity
INV_SKU=$($MCLI board column create --board "$INVENTORY_BOARD" --title "SKU" --type text | $JQ -r '.id')
INV_QTY=$($MCLI board column create --board "$INVENTORY_BOARD" --title "Quantity" --type numbers | $JQ -r '.id')
echo "Inventory columns: SKU=$INV_SKU, Quantity=$INV_QTY"

# Orders: customer, status
ORD_CUSTOMER=$($MCLI board column create --board "$ORDERS_BOARD" --title "Customer" --type text | $JQ -r '.id')
ORD_STATUS=$($MCLI board column create --board "$ORDERS_BOARD" --title "Status" --type status \
  --defaults '{"labels":{"0":"Draft","1":"Ordered","2":"Shipped","3":"Delivered"}}' | $JQ -r '.id')
echo "Orders columns: Customer=$ORD_CUSTOMER, Status=$ORD_STATUS"

# Orders subitems will have: SKU, product name (mirror), price (mirror), qty
# Note: mirror columns are configured via the monday.com UI or API after board-relation is set.
# For the demo we use text columns for the order line subitems.

# ─── Phase 3: Save Semantic Layer (queries & mutations) ──────────────────────

echo ""
echo "=== Phase 3: Define semantic layer ==="

# --- Queries ---

$MCLI query save list_products --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 100) { items { id name column_values { id text value } } } } }"
echo "Saved query: list_products"

$MCLI query save get_inventory --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 500) { items { id name column_values { id text value } } } } }"
echo "Saved query: get_inventory"

$MCLI query save list_orders --query \
  "query(\$boardId: ID!) { boards(ids: [\$boardId]) { items_page(limit: 100) { items { id name column_values { id text value } subitems { id name column_values { id text value } } } } } }"
echo "Saved query: list_orders"

$MCLI query save get_order --query \
  "query(\$itemId: [ID!]!) { items(ids: \$itemId) { id name column_values { id text value } subitems { id name column_values { id text value } } } }"
echo "Saved query: get_order"

# --- Mutations ---

$MCLI mutation save create_product --query \
  "mutation(\$board: ID!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_product"

$MCLI mutation save update_inventory --query \
  "mutation(\$board: ID!, \$item: ID!, \$cols: JSON!) { change_multiple_column_values(board_id: \$board, item_id: \$item, column_values: \$cols) { id } }"
echo "Saved mutation: update_inventory"

$MCLI mutation save create_order --query \
  "mutation(\$board: ID!, \$name: String!, \$cols: JSON!) { create_item(board_id: \$board, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: create_order"

$MCLI mutation save add_order_line --query \
  "mutation(\$parent: ID!, \$name: String!, \$cols: JSON!) { create_subitem(parent_item_id: \$parent, item_name: \$name, column_values: \$cols) { id } }"
echo "Saved mutation: add_order_line"

$MCLI mutation save update_order_status --query \
  "mutation(\$board: ID!, \$item: ID!, \$cols: JSON!) { change_multiple_column_values(board_id: \$board, item_id: \$item, column_values: \$cols) { id } }"
echo "Saved mutation: update_order_status"

# ─── Phase 4: Seed Product & Inventory Data ──────────────────────────────────

echo ""
echo "=== Phase 4: Seed data ==="

# Products. SKU is the board's only text column and Price its only numbers column,
# so --text and --number address them without the caller knowing either column ID.
# Description is long_text, which has no shorthand, so it stays on raw --col.
ITEM_WIDGET=$($MCLI item create --board "$PRODUCTS_BOARD" --name "Widget Pro" \
  --text "WGT-001" \
  --number 29.99 \
  --col "$PROD_DESC"='{"text":"Premium widget with enhanced features"}' | $JQ -r '.id')
echo "Created product: Widget Pro ($ITEM_WIDGET)"

ITEM_GADGET=$($MCLI item create --board "$PRODUCTS_BOARD" --name "Gadget X" \
  --text "GDG-002" \
  --number 49.99 \
  --col "$PROD_DESC"='{"text":"Compact gadget for everyday use"}' | $JQ -r '.id')
echo "Created product: Gadget X ($ITEM_GADGET)"

ITEM_DOOHICK=$($MCLI item create --board "$PRODUCTS_BOARD" --name "Doohickey" \
  --text "DHK-003" \
  --number 9.99 \
  --col "$PROD_DESC"='{"text":"Multi-purpose doohickey"}' | $JQ -r '.id')
echo "Created product: Doohickey ($ITEM_DOOHICK)"

# Inventory — one batch instead of three round-trips. Rows are read from stdin and
# written sequentially, so seeding N SKUs costs one command against monday's
# complexity budget rather than N invocations racing it.
$MCLI item create --board "$INVENTORY_BOARD" - --json <<'ROWS' | $JQ -r '"Inventory: written=\(.written) failed=\(.failed)"'
[{"name": "WGT-001", "text": "WGT-001", "number": 100},
 {"name": "GDG-002", "text": "GDG-002", "number": 50},
 {"name": "DHK-003", "text": "DHK-003", "number": 200}]
ROWS

# ─── Phase 5: Place an Order ─────────────────────────────────────────────────

echo ""
echo "=== Phase 5: Create an order ==="

# Create order as Draft
ORDER_COLS=$(printf '{ "%s": "%s", "%s": {"label": "Draft"} }' "$ORD_CUSTOMER" "Acme Corp" "$ORD_STATUS")
ORDER_ID=$($MCLI mutation run create_order \
  --var board="$ORDERS_BOARD" \
  --var name="ORD-1001" \
  --var cols="$ORDER_COLS" | $JQ -r '.data.create_item.id')
echo "Created order: ORD-1001 ($ORDER_ID) — status: Draft"

# Add order lines as subitems
$MCLI mutation run add_order_line \
  --var parent="$ORDER_ID" \
  --var name="Widget Pro x2" \
  --var cols='{}' | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Line 1: Widget Pro (WGT-001) x2 @ \$29.99"

$MCLI mutation run add_order_line \
  --var parent="$ORDER_ID" \
  --var name="Doohickey x5" \
  --var cols='{}' | $JQ -r '.data.create_subitem.id' > /dev/null
echo "  Line 2: Doohickey (DHK-003) x5 @ \$9.99"

# Transition order to "Ordered"
STATUS_COLS=$(printf '{ "%s": {"label": "Ordered"} }' "$ORD_STATUS")
$MCLI mutation run update_order_status \
  --var board="$ORDERS_BOARD" \
  --var item="$ORDER_ID" \
  --var cols="$STATUS_COLS" | $JQ -r '.data' > /dev/null
echo "Order ORD-1001 status → Ordered"

# ─── Phase 6: Verify ─────────────────────────────────────────────────────────

echo ""
echo "=== Phase 6: Verify ==="
echo "--- Products ---"
$MCLI query run list_products --var boardId="$PRODUCTS_BOARD" --pretty
echo ""
echo "--- Order ORD-1001 ---"
$MCLI query run get_order --var itemId="[$ORDER_ID]" --pretty

echo ""
echo "=== Demo complete ==="
echo ""
echo "Board IDs for further exploration:"
echo "  Products:  $PRODUCTS_BOARD"
echo "  Inventory: $INVENTORY_BOARD"
echo "  Orders:    $ORDERS_BOARD"
