-- R8/R4: protect the canonical Finance consultation-only settlement boundary
-- at PostgreSQL, including direct SQL and late course registration.
-- No Academy settlement, payout, or revenue reconciliation is enabled.

-- Freeze both writers while checking existing facts and installing guards.
LOCK TABLE finance_settlement_instructions, academy_courses IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM finance_settlement_instructions i
    JOIN orders o ON o.id = i.order_id
    JOIN academy_courses c ON c.product_id = o.product_id
  ) THEN
    RAISE EXCEPTION 'academy orders already have consultation settlement instructions';
  END IF;
END;
$$;

-- Both writers serialize on the same canonical product row. The shared
-- NO KEY UPDATE lock avoids a check-then-insert race while preserving FK
-- KEY SHARE reads. The immutable order product_id remains the classifier.
CREATE FUNCTION apgic_finance_academy_settlement_boundary()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  v_product_id uuid;
BEGIN
  SELECT o.product_id INTO v_product_id FROM orders o WHERE o.id = NEW.order_id;
  IF v_product_id IS NOT NULL THEN
    PERFORM 1 FROM products WHERE id = v_product_id FOR NO KEY UPDATE;
    IF EXISTS (SELECT 1 FROM academy_courses c WHERE c.product_id = v_product_id) THEN
      RAISE EXCEPTION 'academy order cannot use consultation settlement';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

-- Alphabetically before the existing finance_settlement_instructions_guard:
-- reject the wrong product before any consultation settlement side effect.
CREATE TRIGGER finance_academy_settlement_boundary_guard
BEFORE INSERT ON finance_settlement_instructions
FOR EACH ROW EXECUTE FUNCTION apgic_finance_academy_settlement_boundary();

CREATE FUNCTION apgic_academy_course_finance_boundary()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM products WHERE id = NEW.product_id FOR NO KEY UPDATE;
  IF EXISTS (
    SELECT 1 FROM finance_settlement_instructions i
    JOIN orders o ON o.id = i.order_id
    WHERE o.product_id = NEW.product_id
  ) THEN
    RAISE EXCEPTION 'cannot register academy course after consultation settlement';
  END IF;
  RETURN NEW;
END;
$$;

-- Reject retroactive classification of a settled consultation as Academy.
CREATE TRIGGER academy_courses_finance_boundary_guard
BEFORE INSERT ON academy_courses
FOR EACH ROW EXECUTE FUNCTION apgic_academy_course_finance_boundary();

