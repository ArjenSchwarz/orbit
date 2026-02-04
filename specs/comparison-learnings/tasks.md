---
references:
    - specs/comparison-learnings/requirements.md
    - specs/comparison-learnings/design.md
    - specs/comparison-learnings/decision_log.md
---
# Comparison Learnings Implementation

## Data Types

- [x] 1. Add LearningCategory type and constants <!-- id:th9obg8 -->
  - Stream: 1
  - Requirements: [1.2](requirements.md#1.2)

- [x] 2. Add VariantLearning struct with JSON tags <!-- id:th9obg9 -->
  - Blocked-by: th9obg8 (Add LearningCategory type and constants)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.3](requirements.md#1.3)

- [x] 3. Add learnings limit constants <!-- id:th9obga -->
  - Blocked-by: th9obg9 (Add VariantLearning struct with JSON tags)
  - Stream: 1
  - Requirements: [6.1](requirements.md#6.1)

- [x] 4. Add Learnings field to Result struct with omitempty <!-- id:th9obgb -->
  - Blocked-by: th9obg9 (Add VariantLearning struct with JSON tags)
  - Stream: 1
  - Requirements: [1.4](requirements.md#1.4), [1.5](requirements.md#1.5)

- [x] 5. Write unit tests for VariantLearning JSON marshaling <!-- id:th9obgc -->
  - Blocked-by: th9obg9 (Add VariantLearning struct with JSON tags), th9obgb (Add Learnings field to Result struct with omitempty)
  - Stream: 1
  - Requirements: [1.1](requirements.md#1.1), [1.5](requirements.md#1.5)

## Validation

- [x] 6. Write unit tests for validateLearnings function <!-- id:th9obgd -->
  - Blocked-by: th9obga (Add learnings limit constants), th9obgb (Add Learnings field to Result struct with omitempty)
  - Stream: 1
  - Owner: agent-validation
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3), [6.5](requirements.md#6.5)

- [x] 7. Implement validateLearnings with field validation <!-- id:th9obge -->
  - Blocked-by: th9obgd (Write unit tests for validateLearnings function)
  - Stream: 1
  - Requirements: [6.1](requirements.md#6.1), [6.3](requirements.md#6.3)

- [x] 8. Add limit enforcement to validateLearnings <!-- id:th9obgf -->
  - Blocked-by: th9obge (Implement validateLearnings with field validation)
  - Stream: 1
  - Requirements: [6.5](requirements.md#6.5)

- [x] 9. Integrate validateLearnings into parseAndValidate <!-- id:th9obgg -->
  - Blocked-by: th9obgf (Add limit enforcement to validateLearnings)
  - Stream: 1
  - Requirements: [6.2](requirements.md#6.2), [6.4](requirements.md#6.4)

## Shared Helpers

- [x] 10. Create learnings.go with GroupLearningsByVariant helper <!-- id:th9obgh -->
  - Blocked-by: th9obg9 (Add VariantLearning struct with JSON tags)
  - Stream: 1
  - Owner: agent-validation
  - Requirements: [3.2](requirements.md#3.2), [4.3](requirements.md#4.3)

- [x] 11. Add SortedVariantIDs helper for deterministic ordering <!-- id:th9obgi -->
  - Blocked-by: th9obgh (Create learnings.go with GroupLearningsByVariant helper)
  - Stream: 1
  - Requirements: [3.2](requirements.md#3.2), [4.3](requirements.md#4.3)

- [x] 12. Write unit tests for learnings helper functions <!-- id:th9obgj -->
  - Blocked-by: th9obgi (Add SortedVariantIDs helper for deterministic ordering)
  - Stream: 1
  - Requirements: [3.2](requirements.md#3.2)

## AI Prompt

- [x] 13. Write tests for learnings section in comparison prompt <!-- id:th9obgk -->
  - Blocked-by: th9obg9 (Add VariantLearning struct with JSON tags)
  - Stream: 2
  - Requirements: [2.1](requirements.md#2.1), [2.6](requirements.md#2.6)

- [x] 14. Update jsonSchema constant with learnings array structure <!-- id:th9obgl -->
  - Blocked-by: th9obgk (Write tests for learnings section in comparison prompt)
  - Stream: 2
  - Requirements: [2.6](requirements.md#2.6)

- [x] 15. Add learnings instructions to buildComparisonPrompt <!-- id:th9obgm -->
  - Blocked-by: th9obgl (Update jsonSchema constant with learnings array structure)
  - Stream: 2
  - Requirements: [2.1](requirements.md#2.1), [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5)

- [x] 16. Add quality guidelines and examples to prompt <!-- id:th9obgn -->
  - Blocked-by: th9obgm (Add learnings instructions to buildComparisonPrompt)
  - Stream: 2
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)

## Markdown Rendering

- [x] 17. Write tests for learnings markdown rendering <!-- id:th9obgo -->
  - Blocked-by: th9obgg (Integrate validateLearnings into parseAndValidate), th9obgj (Write unit tests for learnings helper functions)
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [3.5](requirements.md#3.5)

- [x] 18. Add learnings section to generateMarkdownReport <!-- id:th9obgp -->
  - Blocked-by: th9obgo (Write tests for learnings markdown rendering)
  - Stream: 1
  - Requirements: [3.1](requirements.md#3.1), [3.2](requirements.md#3.2), [3.3](requirements.md#3.3), [3.4](requirements.md#3.4)

- [x] 19. Add stale reference disclaimer to markdown output <!-- id:th9obgq -->
  - Blocked-by: th9obgp (Add learnings section to generateMarkdownReport)
  - Stream: 1
  - Requirements: [3.6](requirements.md#3.6)

## HTML Rendering

- [ ] 20. Add groupLearningsByVariant template function <!-- id:th9obgr -->
  - Blocked-by: th9obgh (Create learnings.go with GroupLearningsByVariant helper)
  - Stream: 1
  - Requirements: [4.3](requirements.md#4.3)

- [ ] 21. Add CSS styles for learnings section <!-- id:th9obgs -->
  - Blocked-by: th9obgr (Add groupLearningsByVariant template function)
  - Stream: 1
  - Requirements: [4.2](requirements.md#4.2), [4.4](requirements.md#4.4), [4.6](requirements.md#4.6)

- [ ] 22. Add default CSS for unknown category badges <!-- id:th9obgt -->
  - Blocked-by: th9obgs (Add CSS styles for learnings section)
  - Stream: 1
  - Requirements: [4.2](requirements.md#4.2)

- [ ] 23. Add learnings section to index.html template <!-- id:th9obgu -->
  - Blocked-by: th9obgt (Add default CSS for unknown category badges)
  - Stream: 1
  - Requirements: [4.1](requirements.md#4.1), [4.3](requirements.md#4.3), [4.5](requirements.md#4.5)

- [ ] 24. Write XSS safety tests for HTML learnings rendering <!-- id:th9obgv -->
  - Blocked-by: th9obgu (Add learnings section to index.html template)
  - Stream: 1
  - Requirements: [4.7](requirements.md#4.7)

## Integration

- [ ] 25. Write integration test for comparison with learnings <!-- id:th9obgw -->
  - Blocked-by: th9obgg (Integrate validateLearnings into parseAndValidate), th9obgn (Add quality guidelines and examples to prompt), th9obgq (Add stale reference disclaimer to markdown output), th9obgv (Write XSS safety tests for HTML learnings rendering)
  - Stream: 1
  - Requirements: [6.2](requirements.md#6.2), [6.4](requirements.md#6.4)

- [ ] 26. Write integration test for graceful degradation on malformed learnings <!-- id:th9obgx -->
  - Blocked-by: th9obgw (Write integration test for comparison with learnings)
  - Stream: 1
  - Requirements: [6.2](requirements.md#6.2), [6.4](requirements.md#6.4)

- [ ] 27. Run full test suite and fix any issues <!-- id:th9obgy -->
  - Blocked-by: th9obgx (Write integration test for graceful degradation on malformed learnings)
  - Stream: 1
  - Requirements: [6.4](requirements.md#6.4)
