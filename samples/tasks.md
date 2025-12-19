---
references:
    - specs/timer-control/requirements.md
    - specs/timer-control/design.md
    - specs/timer-control/decision_log.md
---
# Timer Control

## Foundation

- [ ] 1. Create RunningTimer model
  - Implement struct with id, title, startDate, project, notes, isRunning
  - Add computed elapsedDuration property
  - Implement Codable with CodingKeys for API field mapping (self->id, start_date->startDate, is_running->isRunning)
  - Ensure Sendable conformance
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4)

- [ ] 2. Create ProjectNode model
  - Implement struct with id, title, color, children array
  - Add isLeaf computed property
  - Add allLeafProjects computed property for recursive flattening
  - Implement Codable with CodingKeys (self->id)
  - Ensure Hashable conformance for picker usage
  - Create ProjectHierarchyResponse wrapper struct
  - Requirements: [11.4](requirements.md#11.4)

- [ ] 3. Create TimerPreset model
  - Implement struct with id (UUID), projectId, projectTitle, projectColor, title (optional)
  - Add displayTitle computed property returning title or projectTitle
  - Ensure Codable, Sendable, Hashable conformance
  - Requirements: [8.2](requirements.md#8.2), [8.3](requirements.md#8.3)

- [ ] 4. Create SharedTimerState model
  - Implement struct with isRunning, projectId, projectTitle, projectColor, title, startDate, lastUpdated
  - Add static empty instance
  - Add convenience initializer from optional RunningTimer
  - Requirements: [12.1](requirements.md#12.1)

- [ ] 5. Implement SharedDataStore
  - Create enum with appGroupID constant (group.me.nore.ig.phase)
  - Implement sharedDefaults computed property
  - Implement saveTimerState/getTimerState/clearTimerState methods
  - Implement saveProjectHierarchy/getProjectHierarchy methods
  - Implement isProjectCacheStale with 24-hour TTL per Decision 8
  - Implement getAllLeafProjects helper for entity queries
  - Implement canExecutePreset/recordPresetExecution for 2-second debounce (NFR-1.4)
  - Requirements: [11.2](requirements.md#11.2), [11.3](requirements.md#11.3), [12.1](requirements.md#12.1), [12.2](requirements.md#12.2), [12.3](requirements.md#12.3)

- [ ] 6. Update KeychainService for shared access
  - Add accessGroup parameter with Team ID prefix format (TEAMID.me.nore.ig.phase)
  - Update saveAPIKey to include access group and kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly per Decision 9
  - Update getAPIKey to query with access group
  - Update deleteAPIKey to use access group
  - Note: Existing API keys will require re-entry after migration
  - Requirements: [13.1](requirements.md#13.1), [13.2](requirements.md#13.2), [13.4](requirements.md#13.4)

- [ ] 7. Write unit tests for data models
  - Test RunningTimer Codable encoding/decoding with sample JSON
  - Test ProjectNode hierarchy flattening with nested structure
  - Test SharedTimerState initialization from RunningTimer
  - Test TimerPreset displayTitle behavior
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [11.4](requirements.md#11.4)

- [ ] 8. Write unit tests for SharedDataStore
  - Test timer state persistence and retrieval
  - Test project hierarchy caching and retrieval
  - Test cache staleness detection at boundary
  - Test widget debouncing with rapid calls
  - Use mock UserDefaults or reset between tests
  - Requirements: [12.1](requirements.md#12.1), [12.2](requirements.md#12.2)

## API Integration

- [ ] 9. Add TimingAPIError timer cases
  - Add noProjectOrTitle error case for start validation
  - Add noRunningTimer error case for stop when none running
  - Add projectNotFound error case
  - Update localizedDescription for user-facing messages per NFR-2
  - Requirements: [2.7](requirements.md#2.7)

- [ ] 10. Implement startTimer API method
  - Add startTimer(projectId:title:) async throws -> RunningTimer
  - Validate at least one of projectId or title is provided
  - Build POST request to /time-entries/start with JSON body
  - Include X-Time-Zone header with TimeZone.current.identifier
  - Handle rate limiting via existing RateLimitTracker
  - Decode TimerResponse wrapper and return RunningTimer
  - Requirements: [2.7](requirements.md#2.7), [2.8](requirements.md#2.8)

- [ ] 11. Implement stopTimer API method
  - Add stopTimer() async throws -> RunningTimer
  - Build PUT request to /time-entries/stop
  - Handle 404 by throwing noRunningTimer error
  - Include X-Time-Zone header
  - Return the stopped timer data for duration calculation
  - Requirements: [3.1](requirements.md#3.1)

- [ ] 12. Implement getRunningTimer API method
  - Add getRunningTimer() async throws -> RunningTimer?
  - Build GET request to /time-entries/running
  - Handle 302 redirect automatically via URLSession
  - Return nil for 404 (no running timer - not an error)
  - Handle 401 invalid API key
  - Handle 429 rate limit exceeded
  - Requirements: [1.6](requirements.md#1.6), [1.8](requirements.md#1.8)

- [ ] 13. Implement fetchProjectHierarchy API method
  - Add fetchProjectHierarchy() async throws -> [ProjectNode]
  - Build GET request to /projects/hierarchy
  - Decode ProjectHierarchyResponse and return data array
  - Use reloadIgnoringLocalCacheData cache policy
  - Requirements: [11.1](requirements.md#11.1)

- [ ] 14. Write unit tests for timer API methods
  - Create MockTimerURLProtocol for network isolation
  - Test startTimer success with project only
  - Test startTimer success with title only
  - Test startTimer validation error when both nil
  - Test stopTimer success
  - Test stopTimer when no timer running
  - Test getRunningTimer with running timer
  - Test getRunningTimer with no timer (404)
  - Test fetchProjectHierarchy with nested structure
  - Requirements: [2.7](requirements.md#2.7), [3.1](requirements.md#3.1), [1.6](requirements.md#1.6), [11.1](requirements.md#11.1)

## In-App Timer Control

- [ ] 15. Add timer state properties to DashboardViewModel
  - Add runningTimer: RunningTimer? property
  - Add timerLoadingState: LoadingState<RunningTimer?> property
  - Add projectHierarchy: [ProjectNode] array
  - Add isTimerActionInProgress: Bool for debouncing
  - Add elapsedTimeTick: Int for triggering view updates
  - Add @ObservationIgnored timerUpdateTask: Task for timer updates
  - Requirements: [1.1](requirements.md#1.1), [1.7](requirements.md#1.7)

- [ ] 16. Implement checkRunningTimer in ViewModel
  - Guard against concurrent actions with isTimerActionInProgress
  - Call apiClient.getRunningTimer()
  - Update runningTimer and timerLoadingState
  - Save state to SharedDataStore for widget access
  - Start/stop timer update task based on result
  - Requirements: [1.6](requirements.md#1.6), [12.2](requirements.md#12.2)

- [ ] 17. Implement startTimer in ViewModel
  - Guard against concurrent actions
  - Call apiClient.startTimer with projectId and title
  - Update runningTimer and timerLoadingState on success
  - Save state to SharedDataStore
  - Call WidgetCenter.shared.reloadTimelines
  - Announce to VoiceOver: Timer started for [project name]
  - Start timer update task
  - Requirements: [2.8](requirements.md#2.8), [2.10](requirements.md#2.10), [12.3](requirements.md#12.3)

- [ ] 18. Implement stopTimer in ViewModel
  - Guard against concurrent actions
  - Call apiClient.stopTimer()
  - Calculate duration for VoiceOver announcement
  - Clear runningTimer and update timerLoadingState
  - Save cleared state to SharedDataStore
  - Call WidgetCenter.shared.reloadTimelines
  - Announce to VoiceOver: Timer stopped after [duration]
  - Handle noRunningTimer gracefully with informational message
  - Stop timer update task
  - Requirements: [3.3](requirements.md#3.3), [3.5](requirements.md#3.5), [12.3](requirements.md#12.3)

- [ ] 19. Implement loadProjectHierarchy in ViewModel
  - Check SharedDataStore cache first if not stale
  - If cache valid, use cached hierarchy
  - Otherwise fetch from API and save to SharedDataStore
  - Fall back to stale cache on error
  - Requirements: [11.3](requirements.md#11.3)

- [ ] 20. Implement timer elapsed time updates
  - Create startTimerUpdates() method with Task running every second
  - Increment elapsedTimeTick to trigger @Observable updates
  - Check Task.isCancelled and runningTimer != nil in loop
  - Create stopTimerUpdates() to cancel task
  - Requirements: [1.4](requirements.md#1.4)

- [ ] 21. Create RunningTimerCard view
  - Display project color indicator circle
  - Display project name as headline
  - Display optional title as subheadline if set
  - Display elapsed time in HH:MM:SS format with monospaced font
  - Include stop button with red stop.circle.fill icon
  - Show loading state while stopping
  - Display error via alert on failure
  - Add accessibility label for stop button
  - Requirements: [1.2](requirements.md#1.2), [1.3](requirements.md#1.3), [1.4](requirements.md#1.4), [1.5](requirements.md#1.5)

- [ ] 22. Create ProjectHierarchyPicker view
  - Display hierarchical list of ProjectNode items
  - Track expanded nodes in Set<String> state
  - Create ProjectNodeRow with indentation based on depth
  - Show expand/collapse chevron for parent nodes
  - Show project color indicator and title
  - Show checkmark for selected project
  - Tap leaf to select, tap parent to expand/collapse
  - Add accessibility labels for folder vs leaf items
  - Requirements: [2.3](requirements.md#2.3), [2.4](requirements.md#2.4), [2.5](requirements.md#2.5)

- [ ] 23. Create StartTimerSheet view
  - Present as NavigationStack with Cancel and Start toolbar items
  - Include ProjectHierarchyPicker in Project section
  - Include optional title TextField in Title section
  - Validate canStart: require project OR non-empty title
  - Disable Start button until valid and not loading
  - Show loading overlay while starting
  - Display error via alert on failure
  - Load project hierarchy on appear
  - Dismiss on success
  - Requirements: [2.2](requirements.md#2.2), [2.3](requirements.md#2.3), [2.6](requirements.md#2.6), [2.7](requirements.md#2.7), [2.9](requirements.md#2.9), [2.10](requirements.md#2.10), [2.11](requirements.md#2.11)

- [ ] 24. Update DashboardView for timer control
  - Add RunningTimerCard at top when timer is running (conditionally shown)
  - Add start timer button (FAB or prominent button)
  - Track showStartTimerSheet state
  - Present StartTimerSheet as sheet
  - Call checkRunningTimer when scene becomes active
  - Pass viewModel to sheets and cards
  - Requirements: [1.1](requirements.md#1.1), [1.7](requirements.md#1.7), [2.1](requirements.md#2.1), [2.2](requirements.md#2.2)

- [ ] 25. Write unit tests for ViewModel timer methods
  - Test checkRunningTimer updates state correctly
  - Test startTimer success flow
  - Test startTimer updates SharedDataStore
  - Test stopTimer success flow
  - Test stopTimer handles noRunningTimer gracefully
  - Test loadProjectHierarchy uses cache when valid
  - Test loadProjectHierarchy fetches when stale
  - Use mock APIKeyProvider and URLProtocol
  - Requirements: [1.6](requirements.md#1.6), [2.10](requirements.md#2.10), [3.3](requirements.md#3.3), [11.3](requirements.md#11.3)

## App Intents

- [ ] 26. Create ProjectEntity for App Intents
  - Implement AppEntity with id, title, color properties
  - Set typeDisplayRepresentation to Project
  - Implement displayRepresentation showing title
  - Add convenience initializer from ProjectNode
  - Create ProjectEntityQuery for entity resolution
  - Requirements: [4.2](requirements.md#4.2)

- [ ] 27. Implement ProjectEntityQuery
  - Implement entities(for identifiers:) to resolve by ID from cache
  - Implement suggestedEntities() returning all leaf projects from SharedDataStore
  - Return placeholder entity with Open app to load projects message if cache empty
  - Requirements: [4.2](requirements.md#4.2), [11.5](requirements.md#11.5)

- [ ] 28. Create StartTimerIntent
  - Set title to Start Timing Timer
  - Add optional @Parameter project: ProjectEntity?
  - Add @Parameter timerTitle: String with default empty
  - Validate at least one of project or title provided
  - Create TimingAPIClient and call startTimer
  - Update SharedDataStore and reload widget timelines
  - Return JSON with success, projectName, title, startDate
  - Register with natural language phrases
  - Requirements: [4.1](requirements.md#4.1), [4.2](requirements.md#4.2), [4.3](requirements.md#4.3), [4.4](requirements.md#4.4), [4.5](requirements.md#4.5), [4.6](requirements.md#4.6)

- [ ] 29. Create StopTimerIntent
  - Set title to Stop Timing Timer
  - Create TimingAPIClient and call stopTimer
  - Update SharedDataStore with cleared state
  - Reload widget timelines
  - Calculate duration for result
  - Return JSON with success, projectName, title, duration
  - Handle noRunningTimer with success message not an error
  - Requirements: [5.1](requirements.md#5.1), [5.2](requirements.md#5.2), [5.3](requirements.md#5.3), [5.4](requirements.md#5.4)

- [ ] 30. Create GetRunningTimerIntent
  - Set title to Get Running Timer
  - Create TimingAPIClient and call getRunningTimer
  - If timer running, return JSON with project, title, startDate, elapsedDuration
  - If no timer running, return empty object {}
  - Requirements: [6.1](requirements.md#6.1), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3), [6.4](requirements.md#6.4)

- [ ] 31. Create TimerResult helper structs
  - Create TimerResult struct for start intent (success, projectName, title, startDate)
  - Create TimerStopResult struct for stop intent (success, projectName, title, duration)
  - Ensure Codable conformance for JSON encoding
  - Requirements: [4.5](requirements.md#4.5), [5.3](requirements.md#5.3)

- [ ] 32. Update PhaseShortcuts to register timer intents
  - Add StartTimerIntent to appShortcuts
  - Add StopTimerIntent to appShortcuts
  - Add GetRunningTimerIntent to appShortcuts
  - Configure phrases for Siri discovery
  - Requirements: [4.6](requirements.md#4.6)

- [ ] 33. Write unit tests for App Intents
  - Test StartTimerIntent success flow
  - Test StartTimerIntent validation error
  - Test StopTimerIntent success flow
  - Test StopTimerIntent when no timer running
  - Test GetRunningTimerIntent with running timer
  - Test GetRunningTimerIntent with no timer
  - Test ProjectEntityQuery returns cached projects
  - Requirements: [4.4](requirements.md#4.4), [5.4](requirements.md#5.4), [6.2](requirements.md#6.2), [6.3](requirements.md#6.3)

## Widget Extension

- [ ] 34. Create widget extension target
  - Add phaseWidget target to Xcode project
  - Configure App Group capability (group.me.nore.ig.phase)
  - Configure Keychain Sharing capability with Team ID prefix
  - Add required files: PhaseWidgetBundle.swift
  - Link shared code from main app target
  - Requirements: [13.2](requirements.md#13.2), [13.3](requirements.md#13.3)

- [ ] 35. Create TimerWidgetEntry
  - Implement TimelineEntry with date, presets array, timerState, hasError
  - Presets array contains up to 4 TimerPreset items
  - timerState is optional SharedTimerState from cache
  - Requirements: [7.2](requirements.md#7.2), [9.1](requirements.md#9.1)

- [ ] 36. Create TimerWidgetProvider
  - Implement AppIntentTimelineProvider protocol
  - Create placeholder entry with empty presets
  - Create snapshot by reading current state
  - Create timeline with single entry and .never policy
  - Build presets from configuration.preset1-4
  - Read timerState from SharedDataStore.getTimerState()
  - Requirements: [7.2](requirements.md#7.2), [9.3](requirements.md#9.3)

- [ ] 37. Create TimerWidgetIntent for configuration
  - Create WidgetConfigurationIntent subclass
  - Add 4 preset project parameters using ProjectEntity
  - Add 4 optional preset title string parameters
  - Enable per-instance configuration
  - Requirements: [8.1](requirements.md#8.1), [8.2](requirements.md#8.2), [8.3](requirements.md#8.3), [8.4](requirements.md#8.4)

- [ ] 38. Create StartTimerForPresetIntent
  - Implement AppIntent for widget button taps
  - Set isDiscoverable to false (internal use only)
  - Add projectId and optional title parameters
  - Check SharedDataStore.canExecutePreset for debouncing
  - Record execution time with recordPresetExecution
  - Call apiClient.startTimer
  - Update SharedDataStore on success only (no optimistic updates per Decision 10)
  - Reload widget timelines
  - Requirements: [7.4](requirements.md#7.4), [7.7](requirements.md#7.7), [10.3](requirements.md#10.3)

- [ ] 39. Create TimerWidgetView
  - Show emptyState when presets array is empty with Configure presets message
  - Show presetGrid with 2x2 Grid layout
  - Each preset button uses Button(intent: StartTimerForPresetIntent)
  - Display project color circle and truncated title
  - Highlight active preset if timerState.projectId matches
  - Use containerBackground for widget styling
  - Add accessibility labels: Start timer for [project name]
  - Requirements: [7.2](requirements.md#7.2), [7.3](requirements.md#7.3), [8.5](requirements.md#8.5), [8.6](requirements.md#8.6), [9.1](requirements.md#9.1), [9.2](requirements.md#9.2)

- [ ] 40. Create TimerWidget configuration
  - Create Widget struct with kind = TimerWidget
  - Use AppIntentConfiguration with TimerWidgetIntent
  - Set configurationDisplayName to Timer Presets
  - Set description explaining single-tap timer starts
  - Set supportedFamilies to [.systemSmall] only per Decision 5
  - Requirements: [7.1](requirements.md#7.1)

- [ ] 41. Create PhaseWidgetBundle
  - Implement WidgetBundle with @main attribute
  - Include TimerWidget in body
  - Requirements: [7.1](requirements.md#7.1)

- [ ] 42. Write unit tests for widget components
  - Test TimerWidgetProvider creates correct entry from configuration
  - Test StartTimerForPresetIntent debouncing logic
  - Test StartTimerForPresetIntent does not update state on failure
  - Test TimerWidgetView shows empty state correctly
  - Test TimerWidgetView shows presets in grid
  - Requirements: [7.7](requirements.md#7.7), [8.6](requirements.md#8.6), [10.3](requirements.md#10.3)
