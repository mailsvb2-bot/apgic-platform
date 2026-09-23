.PHONY: canon test guard check

canon:
	python3 tools/canon_lint.py

guard:
	python3 tools/architecture_guard.py

test:
	cd backend && go test ./...

check: canon guard test
