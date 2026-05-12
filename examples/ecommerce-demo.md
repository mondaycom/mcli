# E-Commerce Demo: Products, Inventory & Orders

This demo shows how an LLM agent (or human) can use mcli's generic commands to:

1. **Create boards** with typed columns
2. **Define a semantic layer** of saved queries/mutations with business-domain names
3. **Seed data** using the structured item commands
4. **Place an order** using only the semantic layer (no board IDs in the "business logic")

## Prerequisites

- `mcli auth login` (or `MONDAY_API_TOKEN` set)
- `jq` in PATH

Run the full demo: `./examples/ecommerce-demo.sh`

---

## Phase 1: Create Boards

```sh
mcli board create --name "Products"  --kind public --empty   # → {"id":"..."}
mcli board create --name "Inventory" --kind public --empty
mcli board create --name "Orders"    --kind public --empty
```

Each board starts empty (no default columns/items).

## Phase 2: Define Columns

**Products** — item name is the product name; add SKU, Description, Price:

```sh
mcli board column create --board $PRODUCTS_BOARD --title "SKU"         --type text
mcli board column create --board $PRODUCTS_BOARD --title "Description" --type long_text
mcli board column create --board $PRODUCTS_BOARD --title "Price"       --type numbers
```

**Inventory** — item name is the SKU for quick lookup:

```sh
mcli board column create --board $INVENTORY_BOARD --title "SKU"      --type text
mcli board column create --board $INVENTORY_BOARD --title "Quantity"  --type numbers
```

**Orders** — item name is the order ID; subitems are order lines:

```sh
mcli board column create --board $ORDERS_BOARD --title "Customer" --type text
mcli board column create --board $ORDERS_BOARD --title "Status"   --type status \
  --defaults '{"labels":{"0":"Draft","1":"Ordered","2":"Shipped","3":"Delivered"}}'
```

## Phase 3: Semantic Layer

Saved queries and mutations give business-level names to operations. An LLM can
call `mcli mutation run create_order --var ...` without knowing GraphQL:

### Queries

| Name | Purpose |
|------|---------|
| `list_products` | All products with column values |
| `get_inventory` | Full inventory snapshot |
| `list_orders` | Orders with their line-item subitems |
| `get_order` | Single order detail by item ID |

```sh
mcli query save list_products --query \
  'query($boardId: ID!) { boards(ids: [$boardId]) { items_page(limit:100) { items { id name column_values { id text value } } } } }'
```

### Mutations

| Name | Purpose |
|------|---------|
| `create_product` | Add a product to the catalog |
| `update_inventory` | Adjust stock quantity |
| `create_order` | Create a new order (starts as Draft) |
| `add_order_line` | Add a line-item subitem to an order |
| `update_order_status` | Transition order status |

```sh
mcli mutation save create_order --query \
  'mutation($board: ID!, $name: String!, $cols: JSON!) { create_item(board_id: $board, item_name: $name, column_values: $cols) { id } }'
```

## Phase 4: Seed Data

Using the structured `item create` command (more ergonomic for setup):

```sh
mcli item create --board $PRODUCTS_BOARD --name "Widget Pro" \
  --col "$PROD_SKU"='"WGT-001"' \
  --col "$PROD_PRICE"='"29.99"'
```

Or using the semantic layer:

```sh
mcli mutation run create_product \
  --var board=$PRODUCTS_BOARD \
  --var name="Widget Pro" \
  --var cols='{"sku":"WGT-001","price":"29.99"}'
```

## Phase 5: Place an Order

This is where the semantic layer shines — the agent writes natural JSON in `--var`
and mcli handles the encoding automatically. monday.com's `JSON` scalar expects a
stringified JSON value on the wire, but mcli detects variables declared as `JSON`
in the query and re-encodes them transparently. No double-escaping needed.

```sh
# 1. Create order in Draft status
mcli mutation run create_order \
  --var board=$ORDERS_BOARD \
  --var name="ORD-1001" \
  --var cols='{"customer":"Acme Corp","status":{"label":"Draft"}}'

# 2. Add line items
mcli mutation run add_order_line \
  --var parent=$ORDER_ID \
  --var name="Widget Pro x2" \
  --var cols='{}'

mcli mutation run add_order_line \
  --var parent=$ORDER_ID \
  --var name="Doohickey x5" \
  --var cols='{}'

# 3. Confirm the order
mcli mutation run update_order_status \
  --var board=$ORDERS_BOARD \
  --var item=$ORDER_ID \
  --var cols='{"status":{"label":"Ordered"}}'
```

## Phase 6: Verify

```sh
mcli query run list_products --var boardId=$PRODUCTS_BOARD --pretty
mcli query run get_order --var itemId="[$ORDER_ID]" --pretty
```

## Key Takeaways

- **Generic commands** (`board create`, `item create`) handle setup
- **Saved queries/mutations** create a domain-specific API layer
- **JSON coercion** — variables declared as `JSON` in the query are auto-stringified, so `--var cols='{"key":"val"}'` just works without double-encoding
- **LLM agents** can operate entirely through `mcli mutation run <name>` / `mcli query run <name>` without understanding monday.com internals or wire-format quirks
- The semantic layer is project-local (`.mcli/` directory) and version-controllable
