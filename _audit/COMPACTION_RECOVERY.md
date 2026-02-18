# Context Compaction Recovery Guide

If context is lost mid-audit, follow these steps:

## 1. Check Current Progress
Read `_audit/PHASE_PROGRESS.md` to see which phases are complete.

## 2. Check Master Tracker
Read `_audit/MASTER_TRACKER.md` for all issues found so far.

## 3. Resume From Last Incomplete Phase
Each phase writes its output to a dedicated file. If a phase file exists but is incomplete, re-run that phase.

## 4. Key Files to Re-Read
- `_audit/ARCHITECTURE_MAP.md` — System inventory (Phase 1)
- `_audit/FEATURE_MATRIX.md` — Feature completeness (Phase 1)
- `_audit/WIRING_MAP.md` — FE->BE connections (Phase 3)
- `_audit/REVERSE_WIRING_MAP.md` — BE->FE connections (Phase 3B)

## 5. Phase Dependencies
- Phase 0: Independent
- Phase 1: Depends on Phase 0
- Phases 2-6: All depend on Phase 1 (can run in parallel)
- Phase 7: Depends on all of Phases 2-6
