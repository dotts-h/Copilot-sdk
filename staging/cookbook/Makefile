# cookbook — repo gates. See docs/SPEC.md for the contracts these enforce.

PLUGIN := plugins/cookbook

.PHONY: lint evals doctor

## lint: bash -n every script + secret/date scans
lint:
	bash scripts/lint.sh

## evals: run every recipe's evals (fixture repos in mktemp dirs; must all pass)
evals:
	@status=0; \
	for e in $(PLUGIN)/recipes/*/evals/*.sh; do \
		echo "--- $$e"; \
		bash "$$e" || status=1; \
	done; \
	exit $$status

## doctor: run installed-recipe doctors against TARGET (lock-driven; --all survey without a lock)
doctor:
	@[ -n "$(TARGET)" ] || { echo "usage: make doctor TARGET=/path/to/repo"; exit 2; }
	bash $(PLUGIN)/skills/recipe-doctor/scripts/run-doctors.sh "$(TARGET)"
