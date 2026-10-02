-- Repair legacy/baselined installations where 000018 was registered as
-- applied but its order ownership snapshot columns were absent physically.
-- Keep historical pre-product orders untouched; product-linked orders can be
-- reconstructed from the immutable product snapshot already referenced by order.product_id.

ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS product_owner_ref text,
  ADD COLUMN IF NOT EXISTS author_refs text[];

UPDATE orders AS order_row
   SET product_owner_ref = COALESCE(
         order_row.product_owner_ref,
         lower(product.owner_type) || '/' || product.owner_id::text
       ),
       author_refs = COALESCE(order_row.author_refs, product.author_refs)
  FROM products AS product
 WHERE order_row.product_id = product.id
   AND (order_row.product_owner_ref IS NULL OR order_row.author_refs IS NULL);

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
      FROM orders
     WHERE product_id IS NOT NULL
       AND (
         product_owner_ref IS NULL
         OR btrim(product_owner_ref) = ''
         OR author_refs IS NULL
         OR cardinality(author_refs) = 0
       )
  ) THEN
    RAISE EXCEPTION 'product-linked order ownership snapshot could not be reconciled';
  END IF;

  IF NOT EXISTS (
    SELECT 1
      FROM pg_constraint
     WHERE conrelid = 'orders'::regclass
       AND conname = 'orders_product_owner_ref_nonblank'
  ) THEN
    ALTER TABLE orders
      ADD CONSTRAINT orders_product_owner_ref_nonblank
        CHECK (product_owner_ref IS NULL OR btrim(product_owner_ref) <> '');
  END IF;

  IF NOT EXISTS (
    SELECT 1
      FROM pg_constraint
     WHERE conrelid = 'orders'::regclass
       AND conname = 'orders_author_refs_nonempty'
  ) THEN
    ALTER TABLE orders
      ADD CONSTRAINT orders_author_refs_nonempty
        CHECK (author_refs IS NULL OR cardinality(author_refs) > 0);
  END IF;
END;
$$;