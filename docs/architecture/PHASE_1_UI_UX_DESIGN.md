# PHASE 1 — UI/UX DESIGN
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: UI/UX Architecture Team
- **Last Updated**: 2026-01-04

---

## 1. UI Philosophy

### 1.1 Core Design Principles

#### COMMAND-GRADE INTERFACE
This is not consumer software. This is command infrastructure.

| Principle | Implementation |
|-----------|---------------|
| **Zero Ambiguity** | Every button, label, and status must be instantly clear. No clever copy. No ambiguous icons. |
| **Fail-Safe Design** | Destructive actions require confirmation. Critical actions require authentication. |
| **Stress-Resistant** | Works under pressure: large touch targets, high contrast, no hover-only interactions. |
| **Offline-Aware** | Always shows sync status. Never fails silently. Degraded mode clearly indicated. |
| **Audit-Visible** | Every screen shows who is logged in, what role, what jurisdiction, what time. |

#### LOW-LATENCY REQUIREMENTS
| Metric | Target |
|--------|--------|
| Initial page load | < 2 seconds |
| Navigation between views | < 500ms |
| Search results | < 1 second (local), < 3 seconds (district-wide) |
| Form submission feedback | Immediate (< 100ms) |
| Offline indicator | < 200ms after disconnect |

#### STRESS-SAFE DESIGN
```
HIGH-STRESS SCENARIO: Riot situation, multiple FIRs being filed

DESIGN RESPONSES:
• Large touch targets (minimum 44x44px, recommended 56x56px)
• High contrast mode always available
• Keyboard shortcuts for all critical actions
• Voice input for FIR dictation
• Auto-save every 30 seconds
• Confirmation dialogs for destructive actions only
• No animations that block interaction
• Error messages include remediation steps
```

### 1.2 Accessibility Standards

| Standard | Requirement |
|----------|-------------|
| WCAG Level | AA minimum, AAA for critical functions |
| Color Contrast | 4.5:1 minimum for text, 3:1 for large text |
| Keyboard Navigation | Full functionality without mouse |
| Screen Reader | All elements properly labeled |
| Text Scaling | Support up to 200% without horizontal scroll |
| Motion | Reduced motion mode available |

### 1.3 Visual Design System

```
COLOR PALETTE (DARK MODE PRIMARY):
─────────────────────────────────
Background Primary:    #0D1117 (near black)
Background Secondary:  #161B22 (dark gray)
Background Tertiary:   #21262D (medium gray)
Border:               #30363D (light gray)
Text Primary:         #E6EDF3 (near white)
Text Secondary:       #8B949E (muted)

SEMANTIC COLORS:
───────────────
Success:    #238636 (green)
Warning:    #9E6A03 (amber)
Error:      #DA3633 (red)
Info:       #1F6FEB (blue)
Critical:   #FF6B6B (bright red)

ROLE INDICATORS:
───────────────
Constable:       #4A9EFF (blue)
Station Officer: #36B37E (green)
District Admin:  #FFAB00 (amber)
State HQ:        #9B59B6 (purple)
Central:         #E74C3C (red)

TYPOGRAPHY:
──────────
Primary Font:    Inter (system fallback: -apple-system, sans-serif)
Monospace:       JetBrains Mono (for IDs, codes, logs)
Size Scale:      12px / 14px / 16px / 18px / 24px / 32px
Line Height:     1.5 for body, 1.2 for headings
```

---

## 2. Role-Based UI Views

### 2.1 Role Hierarchy & Access Matrix

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ROLE HIERARCHY                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  CENTRAL MINISTRY                                                           │
│  ├── Home Secretary                                                         │
│  ├── Joint Secretary                                                        │
│  └── Central Analyst                                                        │
│                                                                             │
│  STATE HQ                                                                   │
│  ├── Director General of Police (DGP)                                       │
│  ├── Additional DGP                                                         │
│  ├── Inspector General (IG)                                                 │
│  └── State Intelligence Officer                                             │
│                                                                             │
│  DISTRICT HQ                                                                │
│  ├── Superintendent of Police (SP) / Deputy Commissioner (DCP)              │
│  ├── Additional SP                                                          │
│  ├── Deputy SP (DSP)                                                        │
│  └── District Admin                                                         │
│                                                                             │
│  POLICE STATION                                                             │
│  ├── Station House Officer (SHO) / Inspector                                │
│  ├── Sub-Inspector (SI)                                                     │
│  ├── Assistant Sub-Inspector (ASI)                                          │
│  ├── Head Constable                                                         │
│  └── Constable                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Access Matrix

| Module | Constable | SHO | District Admin | State HQ | Central |
|--------|-----------|-----|----------------|----------|---------|
| FIR Create | ✓ | ✓ | ✓ | Read | Read |
| FIR Edit | Own | Station | District | State | Read |
| Case Assign | - | ✓ | ✓ | ✓ | - |
| Evidence Add | ✓ | ✓ | ✓ | - | - |
| Evidence Delete | - | - | Audit | Audit | Audit |
| Personnel View | Self | Station | District | State | National |
| Personnel Edit | - | - | ✓ | ✓ | - |
| Analytics | Station | Station | District | State | National |
| Intelligence | - | Limited | District | Full | Full |
| National Alerts | Read | Read | Read | Manage | Manage |
| System Config | - | - | Limited | Full | Full |

---

## 3. Textual Wireframes

### 3.1 Constable View - Main Dashboard

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NPDMS │ PS: Koramangala │ Constable Ramesh Kumar │ Online ● │ 04-Jan-2026 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │ [+ NEW FIR]      │  │ [📋 MY TASKS]    │  │ [🔍 SEARCH]      │          │
│  │                  │  │                  │  │                  │          │
│  │ Register new     │  │ Pending: 5       │  │ FIR, Person,     │          │
│  │ complaint/FIR    │  │ Due today: 2     │  │ Vehicle, Case    │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  TODAY'S DUTY ASSIGNMENT                                                    │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Shift: Day (0600-1400)     Beat: Koramangala 4th Block                    │
│  Reporting To: SI Suresh    Vehicle: KA-01-P-1234 (Patrol)                 │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  MY ASSIGNED CASES (3)                                                      │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ FIR/2026/KOR/00142 │ Theft │ Pending │ Due: 06-Jan │ [View] [Update]│   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ FIR/2026/KOR/00138 │ Missing Person │ Active │ [View] [Update]      │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ FIR/2026/KOR/00129 │ Assault │ Court Pending │ [View]               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  STATION ALERTS (2)                                                         │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ⚠️  LOOKOUT: White Maruti Swift KA-05-XX-1234 - Theft suspect             │
│  📢  NOTICE: Station meeting at 1400 hours today                           │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  QUICK ACTIONS                                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  [📸 Evidence Photo]  [🎤 Voice Note]  [📍 Mark Location]  [📞 Emergency]  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
│ [Home] │ [Cases] │ [Duty] │ [Armoury] │ [Profile] │                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Station House Officer (SHO) - Command Dashboard

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NPDMS │ PS: Koramangala │ SHO Insp. Sharma │ Online ● │ 04-Jan-2026 14:32 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────── STATION STATUS ───────────┐  ┌────── TODAY'S METRICS ──────┐ │
│  │                                       │  │                             │ │
│  │  Officers on Duty:  12/18            │  │  FIRs Filed:        7       │ │
│  │  Vehicles Active:   4/6              │  │  Cases Closed:      2       │ │
│  │  Cells Occupied:    3/8              │  │  Arrests:           1       │ │
│  │  Pending Tasks:     23               │  │  Challans Issued:   15      │ │
│  │                                       │  │  Public Complaints: 4       │ │
│  │  [Manage Duty] [Vehicle Log]         │  │                             │ │
│  └───────────────────────────────────────┘  └─────────────────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  PRIORITY ITEMS REQUIRING ATTENTION                                         │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  🔴 CRITICAL (2)                                                            │
│  ├── FIR/2026/KOR/00145 - Robbery - Investigation stalled - [REVIEW]       │
│  └── Court Summons - Case 00098 - Hearing tomorrow - [PREPARE]             │
│                                                                             │
│  🟡 URGENT (5)                                                              │
│  ├── Evidence submission pending - 3 cases - [VIEW ALL]                    │
│  ├── Witness statement required - FIR 00142 - [ASSIGN]                     │
│  └── + 3 more items                                                         │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  PERSONNEL OVERVIEW                                                         │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌──────────────┬──────────┬─────────┬──────────┬─────────────────────────┐│
│  │ Name         │ Rank     │ Status  │ Cases    │ Action                  ││
│  ├──────────────┼──────────┼─────────┼──────────┼─────────────────────────┤│
│  │ Ramesh Kumar │ Const    │ On Duty │ 3        │ [View] [Assign]         ││
│  │ Suresh P     │ SI       │ On Duty │ 8        │ [View] [Assign]         ││
│  │ Anita Rao    │ ASI      │ Leave   │ 5        │ [View] [Reassign Cases] ││
│  │ + 15 more    │          │         │          │ [View All Personnel]    ││
│  └──────────────┴──────────┴─────────┴──────────┴─────────────────────────┘│
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  STATION MAP & DEPLOYMENT                                                   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    [INTERACTIVE BEAT MAP]                            │   │
│  │                                                                      │   │
│  │    🚔 Patrol 1 (Beat A)     🚔 Patrol 2 (Beat B)                    │   │
│  │                                                                      │   │
│  │    📍 Recent Incident       📍 Active Case Location                 │   │
│  │                                                                      │   │
│  │    [Full Map] [Deploy] [Reassign] [Historical]                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Dashboard] [FIRs] [Cases] [Personnel] [Armoury] [Vehicles] [Reports]      │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 District Admin - District Command Center

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NPDMS │ District: Bengaluru Urban │ SP Ramachandran │ Online ● │ 04-Jan-26│
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──── DISTRICT OVERVIEW ────┐  ┌──── LIVE STATISTICS ─────────────────┐  │
│  │                           │  │                                       │  │
│  │  Stations: 42/42 Online   │  │  ┌─────────────────────────────────┐ │  │
│  │  Personnel: 2,847         │  │  │      TODAY    │   THIS WEEK     │ │  │
│  │  Active Cases: 12,456     │  │  ├─────────────────────────────────┤ │  │
│  │  Pending FIRs: 234        │  │  │ FIRs:    127  │   FIRs:    892  │ │  │
│  │                           │  │  │ Arrests:  23  │   Arrests: 156  │ │  │
│  │  Critical Alerts: 3       │  │  │ Closed:   45  │   Closed:  312  │ │  │
│  │  [Station Status Map]     │  │  └─────────────────────────────────┘ │  │
│  └───────────────────────────┘  └───────────────────────────────────────┘  │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  CRITICAL ALERTS                                                            │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  🔴 [FLASH] Inter-state gang alert - 3 suspects - Action Required          │
│      Source: State HQ │ Issued: 14:15 │ [View Details] [Acknowledge]       │
│                                                                             │
│  🔴 [URGENT] Koramangala PS - Officer injured - Backup dispatched          │
│      Incident: 13:45 │ Status: Responding │ [View] [Coordinate]            │
│                                                                             │
│  🟡 [NOTICE] Court hearing backlog increasing - 15 stations affected       │
│      Analysis: AI-generated │ [View Report] [Action Plan]                  │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  STATION PERFORMANCE MATRIX                                                 │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────┬────────┬────────┬────────┬────────┬─────────────────┐ │
│  │ Station         │ FIRs   │Pending │ Closed │ Rating │ Action          │ │
│  ├─────────────────┼────────┼────────┼────────┼────────┼─────────────────┤ │
│  │ Koramangala     │ 12     │ 8      │ 4      │ ★★★★☆  │ [Details]       │ │
│  │ Indiranagar     │ 8      │ 3      │ 5      │ ★★★★★  │ [Details]       │ │
│  │ Whitefield      │ 15     │ 12     │ 3      │ ★★★☆☆  │ [Review] ⚠️     │ │
│  │ HSR Layout      │ 6      │ 2      │ 4      │ ★★★★☆  │ [Details]       │ │
│  │ + 38 more       │        │        │        │        │ [View All]      │ │
│  └─────────────────┴────────┴────────┴────────┴────────┴─────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  CRIME ANALYTICS                                                            │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────┐  ┌─────────────────────────────────┐  │
│  │   CRIME HEATMAP (7 DAYS)        │  │   CRIME TYPE DISTRIBUTION       │  │
│  │   ┌─────────────────────────┐   │  │   ┌─────────────────────────┐   │  │
│  │   │ [Interactive Map]       │   │  │   │ Theft        ████████ 34%│  │  │
│  │   │                         │   │  │   │ Assault      █████    21%│  │  │
│  │   │ High ▓▓▓  Med ▒▒▒       │   │  │   │ Cyber Crime  ████     17%│  │  │
│  │   │ Low  ░░░                │   │  │   │ Traffic      ███      12%│  │  │
│  │   └─────────────────────────┘   │  │   │ Other        ████     16%│  │  │
│  │   [Expand] [Historical]         │  │   └─────────────────────────┘   │  │
│  └─────────────────────────────────┘  └─────────────────────────────────┘  │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  AI INSIGHTS (Advisory Only)                                                │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  💡 Predicted high-risk area: Whitefield IT Park (evening hours)           │
│     Confidence: 78% │ Based on: Historical + Event data │ [Details]        │
│                                                                             │
│  💡 Resource optimization: Suggest 2 additional patrols in HSR Layout      │
│     Potential impact: -15% response time │ [Evaluate] [Dismiss]            │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│[Dashboard][Stations][Cases][Personnel][Intelligence][Analytics][Reports]   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.4 State HQ - State Command Dashboard

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NPDMS │ State: Karnataka │ DGP Office │ Online ● │ 04-Jan-2026 14:45      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─── STATE STATUS ────┐  ┌─── CRITICAL METRICS ───┐  ┌─── SYNC STATUS ──┐│
│  │                     │  │                         │  │                   ││
│  │ Districts: 31/31 ●  │  │ Active Cases: 145,678  │  │ Central: ● Synced ││
│  │ Stations: 1,012     │  │ Officers: 89,456       │  │ Last: 2 min ago   ││
│  │ Personnel: 89,456   │  │ Pending Court: 23,456  │  │                   ││
│  │ Vehicles: 4,567     │  │ Critical Alerts: 7     │  │ Districts: 31/31  ││
│  │                     │  │ Inter-state Cases: 234 │  │ Stations: 1,009   ││
│  │ [Full Status]       │  │ [Detailed View]        │  │ [Sync Details]    ││
│  └─────────────────────┘  └─────────────────────────┘  └───────────────────┘│
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  COMMAND ALERTS (PRIORITY ORDER)                                            │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  🔴 [CENTRAL-FLASH] National terror alert - Level Orange - All districts   │
│      Issued: Home Ministry 12:00 │ Expires: 72h │ [Ack: 28/31] [Details]   │
│                                                                             │
│  🔴 [STATE-URGENT] Major accident NH-44 - Multi-agency response active     │
│      Districts: Tumkur, Hassan │ Status: Ongoing │ [Command Center]        │
│                                                                             │
│  🟡 [INTEL] Cross-border smuggling network identified - 3 districts        │
│      Source: State Intelligence │ Classification: SECRET │ [Access]        │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  DISTRICT PERFORMANCE OVERVIEW                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                     [STATE MAP WITH DISTRICTS]                       │   │
│  │                                                                      │   │
│  │    Color-coded by:  ○ Crime Rate  ○ Case Clearance  ○ Response Time │   │
│  │                                                                      │   │
│  │    [Bengaluru Urban] [Mysuru] [Mangaluru] [Hubli-Dharwad] [...]     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INTER-STATE COORDINATION                                                   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Active Cross-Border Cases: 234                                      │    │
│  │ ├── With Tamil Nadu: 89 cases │ [View] [Coordinate]                │    │
│  │ ├── With Kerala: 56 cases │ [View] [Coordinate]                    │    │
│  │ ├── With Maharashtra: 45 cases │ [View] [Coordinate]               │    │
│  │ └── With Andhra Pradesh: 44 cases │ [View] [Coordinate]            │    │
│  │                                                                     │    │
│  │ Pending Extradition Requests: 12 │ [Manage]                        │    │
│  │ Shared Intelligence Reports: 34 (this month) │ [View]              │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INTELLIGENCE DASHBOARD                                                     │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────┐  ┌─────────────────────────────────┐  │
│  │   CRIMINAL NETWORK GRAPH        │  │   TREND ANALYSIS                │  │
│  │   ┌─────────────────────────┐   │  │                                 │  │
│  │   │    [Neo4j Graph View]   │   │  │   Cyber Crime:    ↑ 23%        │  │
│  │   │                         │   │  │   Property Crime: ↓ 8%         │  │
│  │   │    Active Networks: 12  │   │  │   Violent Crime:  → 2%         │  │
│  │   │    Key Nodes: 145       │   │  │   Drug Offenses:  ↑ 15%        │  │
│  │   └─────────────────────────┘   │  │                                 │  │
│  │   [Expand] [Analysis]           │  │   [Detailed Report]             │  │
│  └─────────────────────────────────┘  └─────────────────────────────────┘  │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│[Command][Districts][Intelligence][Personnel][Operations][Analytics][Config]│
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.5 Central Ministry - National Command Dashboard

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NPDMS │ CENTRAL HOME MINISTRY │ Joint Secretary │ SECURE ● │ 04-Jan-2026 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────── NATIONAL STATUS ─────────────────────────────────┐ │
│  │                                                                        │ │
│  │  States Connected: 28/28 + 8 UTs    │   National Alert Level: ORANGE  │ │
│  │  Districts Online: 766/766          │   Active Directives: 12         │ │
│  │  Stations Reporting: 17,234/17,456  │   Last Sync: 2 minutes ago      │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  NATIONAL COMMAND                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │  [🔴 ISSUE NATIONAL ALERT]  [📢 BROADCAST DIRECTIVE]  [🔍 LOOKOUT]   │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  ACTIVE NATIONAL ALERTS                                                     │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  🔴 NAT-2026-0042 │ Terror Alert - Level Orange │ All States              │
│     Issued: 04-Jan 12:00 │ Expires: 07-Jan │ Ack: 28/36 │ [Manage]        │
│                                                                             │
│  🟡 NAT-2026-0041 │ Cyber Fraud Ring │ 12 States │ Active Investigation   │
│     Issued: 02-Jan │ Coordination: CBI │ [View] [Coordinate]              │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  NATIONAL CRIME MAP                                                         │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                                                                        │ │
│  │                    [INDIA MAP - INTERACTIVE]                          │ │
│  │                                                                        │ │
│  │   View: ○ Crime Rate  ○ Case Clearance  ○ Resource Distribution      │ │
│  │   Filter: [All Crimes ▼] [Last 7 Days ▼] [All Severity ▼]            │ │
│  │                                                                        │ │
│  │   Click state for details                                             │ │
│  │                                                                        │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  STATE-WISE SUMMARY                                                         │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌───────────────┬──────────┬───────────┬──────────┬───────────┬────────┐ │
│  │ State         │ Cases    │ Clearance │ Critical │ Sync      │ Action │ │
│  ├───────────────┼──────────┼───────────┼──────────┼───────────┼────────┤ │
│  │ Maharashtra   │ 234,567  │ 67%       │ 23       │ ● Online  │ [View] │ │
│  │ Uttar Pradesh │ 198,456  │ 58%       │ 45       │ ● Online  │ [View] │ │
│  │ Karnataka     │ 145,678  │ 72%       │ 7        │ ● Online  │ [View] │ │
│  │ Tamil Nadu    │ 134,567  │ 75%       │ 5        │ ● Online  │ [View] │ │
│  │ + 32 more     │          │           │          │           │[All]   │ │
│  └───────────────┴──────────┴───────────┴──────────┴───────────┴────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INTER-STATE INTELLIGENCE                                                   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────┐  ┌─────────────────────────────────┐  │
│  │   CROSS-STATE CRIMINAL NETWORKS │  │   NATIONAL WANTED LIST          │  │
│  │   ┌─────────────────────────┐   │  │                                 │  │
│  │   │ [Graph Visualization]   │   │  │   Total: 12,456                 │  │
│  │   │                         │   │  │   High Priority: 234            │  │
│  │   │   Networks: 45          │   │  │   Captured (MTD): 89            │  │
│  │   │   Key Figures: 234      │   │  │                                 │  │
│  │   │   States Affected: 18   │   │  │   [Search] [Add] [Broadcast]   │  │
│  │   └─────────────────────────┘   │  │                                 │  │
│  │   [Full Analysis]               │  │   [View All]                    │  │
│  └─────────────────────────────────┘  └─────────────────────────────────┘  │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Command] [States] [Intelligence] [Alerts] [Policy] [Analytics] [Audit]    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.6 FIR Entry Screen (Multi-Modal)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ NEW FIR REGISTRATION │ PS: Koramangala │ SI Suresh │ 04-Jan-2026 15:00    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  FIR Number: KOR/2026/00148 (Auto-generated)                               │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INPUT METHOD                                                               │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  [📝 Type]  [🎤 Voice]  [📷 Scan Handwritten]  [📄 Upload Document]        │
│       ▲                                                                     │
│       └── Currently Active                                                  │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  COMPLAINANT DETAILS                                                        │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Name*:        [_______________________________________]                   │
│  Father's Name:[_______________________________________]                   │
│  Address*:     [_______________________________________]                   │
│                [_______________________________________]                   │
│  Phone*:       [_______________]  Alt Phone: [_______________]             │
│  ID Type:      [Aadhaar ▼]        ID Number: [________________]            │
│  Age*:         [___] Gender*: [Male ▼]                                     │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INCIDENT DETAILS                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Date of Incident*: [04-Jan-2026]  Time: [14:30]  ○ Approximate            │
│                                                                             │
│  Location*:                                                                 │
│  [_______________________________________] [📍 Mark on Map]                │
│                                                                             │
│  Beat/Area:    [Koramangala 4th Block ▼]                                   │
│                                                                             │
│  Offence Category*: [Property Crime ▼]                                     │
│  Offence Type*:     [Theft ▼]                                              │
│                                                                             │
│  IPC Sections (AI Suggested):                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ☑ Section 379 - Theft                                               │   │
│  │ ☐ Section 380 - Theft in dwelling house (AI Confidence: 45%)        │   │
│  │ [+ Add Section Manually]                                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ⚠️ AI suggestions are advisory. Officer must verify applicability.        │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INCIDENT DESCRIPTION*                                                      │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                                                                      │   │
│  │ [Rich text editor - 500 words minimum recommended]                  │   │
│  │                                                                      │   │
│  │ The complainant reports that on 04-Jan-2026 at approximately        │   │
│  │ 14:30 hours, unknown person(s) entered his residence at...          │   │
│  │                                                                      │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  Characters: 234/500 minimum                                               │
│                                                                             │
│  AI-Extracted Entities (Verify):                                           │
│  ├── Location: "residence at 4th Block, Koramangala" ✓                    │
│  ├── Time: "14:30 hours" ✓                                                │
│  ├── Suspects: None identified                                            │
│  └── Property: [Click to add stolen items]                                │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  ACCUSED/SUSPECT DETAILS (if known)                                        │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ○ Unknown    ● Known/Described                                            │
│                                                                             │
│  [+ Add Suspect]                                                           │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  PROPERTY INVOLVED                                                          │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌──────────────┬────────────────┬─────────────┬───────────────┐           │
│  │ Item         │ Description    │ Est. Value  │ Action        │           │
│  ├──────────────┼────────────────┼─────────────┼───────────────┤           │
│  │ Mobile Phone │ iPhone 15 Pro  │ ₹1,50,000   │ [Edit][Del]   │           │
│  │ Cash         │ Indian Rupees  │ ₹25,000     │ [Edit][Del]   │           │
│  └──────────────┴────────────────┴─────────────┴───────────────┘           │
│  Total Estimated Value: ₹1,75,000                                          │
│  [+ Add Item]                                                              │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  INITIAL EVIDENCE                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  [📷 Add Photo] [🎥 Add Video] [📄 Add Document] [🎤 Add Audio]            │
│                                                                             │
│  Attached: 2 items                                                         │
│  ├── crime_scene_01.jpg (2.3 MB) - Uploaded                               │
│  └── statement_audio.mp3 (5.1 MB) - Uploading... 67%                      │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ☑ I verify that the above information is recorded accurately as     │   │
│  │   stated by the complainant.                                        │   │
│  │                                                                      │   │
│  │ Recording Officer: SI Suresh (Badge: KAR-SI-4567)                   │   │
│  │ Biometric Verification: [Verify Fingerprint]                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  [Save Draft]                    [Preview]                    [Register FIR]│
│                                                                             │
│  Auto-saved: 14:58:32                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.7 Case Management Screen

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ CASE MANAGEMENT │ FIR/2026/KOR/00142 │ Theft │ Status: UNDER INVESTIGATION│
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──── CASE SUMMARY ─────────────────────────────────────────────────────┐ │
│  │                                                                        │ │
│  │  FIR Number:     KOR/2026/00142        │ Registered: 02-Jan-2026      │ │
│  │  Offence:        Theft (IPC 379)       │ Station: Koramangala PS      │ │
│  │  Complainant:    Rajesh Kumar          │ IO: SI Suresh                │ │
│  │  Status:         Under Investigation   │ Court: Not yet filed         │ │
│  │  Priority:       Normal                │ SLA: 12 days remaining       │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [📋 Case Diary] [👥 Persons] [📦 Evidence] [📄 Documents] [⏱️ Timeline]   │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                      ▲                                      │
│                                      └── Active Tab                         │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  CASE DIARY                                                                 │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  [+ Add Entry]                                        [Filter ▼] [Export]  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 04-Jan-2026 14:30 │ SI Suresh                                       │   │
│  │ ─────────────────────────────────────────────────────────────────── │   │
│  │ Visited scene. Collected CCTV footage from neighboring shop.       │   │
│  │ Owner Mr. Sharma provided footage from 14:00-15:00 hours.          │   │
│  │ Footage shows suspect entering premises. Face partially visible.   │   │
│  │                                                                      │   │
│  │ Next Action: Send footage for enhancement. Canvas nearby shops.    │   │
│  │                                                                      │   │
│  │ Attachments: cctv_footage.mp4                       [View Details]  │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ 03-Jan-2026 10:00 │ Const. Ramesh                                   │   │
│  │ ─────────────────────────────────────────────────────────────────── │   │
│  │ Recorded statements from two witnesses:                            │   │
│  │ 1. Mrs. Lakshmi (neighbor) - Saw unknown person near gate          │   │
│  │ 2. Mr. Auto Driver - Dropped person matching description           │   │
│  │                                                                      │   │
│  │ Attachments: witness_stmt_1.pdf, witness_stmt_2.pdf [View Details]  │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ 02-Jan-2026 15:30 │ SI Suresh                                       │   │
│  │ ─────────────────────────────────────────────────────────────────── │   │
│  │ FIR registered. Initial investigation commenced.                   │   │
│  │ Scene visited, photographs taken. Fingerprints lifted.             │   │
│  │                                                                      │   │
│  │ Attachments: scene_photos.zip, fingerprint_01.jpg   [View Details]  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  AI ANALYSIS (Advisory)                                                     │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 🔗 SIMILAR CASES DETECTED (3)                                       │   │
│  │                                                                      │   │
│  │ • FIR/2026/IND/00098 - Similar MO - 85% match - Indiranagar PS     │   │
│  │ • FIR/2025/KOR/02345 - Same locality - 72% match - Koramangala PS  │   │
│  │ • FIR/2025/HSR/00567 - Similar time pattern - 68% match - HSR PS   │   │
│  │                                                                      │   │
│  │ [View Connections] [Link Cases] [Dismiss]                          │   │
│  │                                                                      │   │
│  │ ⚠️ This is AI-generated analysis. Verify before taking action.      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  ACTIONS                                                                    │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  [Transfer Case] [Request Assistance] [Escalate] [Close Case] [To Court]  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.8 Evidence Handling Screen

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ EVIDENCE MANAGEMENT │ Case: FIR/2026/KOR/00142 │ Chain of Custody Active  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  [+ Register New Evidence]  [📥 Bulk Upload]  [🔍 Search Evidence]         │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  EVIDENCE REGISTRY (8 items)                                                │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ EVD-2026-KOR-00142-001 │ Physical │ Fingerprint Sample             │   │
│  │ ─────────────────────────────────────────────────────────────────── │   │
│  │ Collected: 02-Jan-2026 15:45 │ By: SI Suresh                       │   │
│  │ Location: Crime scene - Door handle                                 │   │
│  │ Status: 🔬 At Forensic Lab │ Expected: 10-Jan-2026                 │   │
│  │ Chain: Collected → Sealed → Submitted to Lab                       │   │
│  │                                                                      │   │
│  │ Integrity: ✓ Verified │ Hash: sha256:a3b4c5...                     │   │
│  │                                   [View Chain] [Request Status]     │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ EVD-2026-KOR-00142-002 │ Digital │ CCTV Footage                    │   │
│  │ ─────────────────────────────────────────────────────────────────── │   │
│  │ Collected: 04-Jan-2026 14:30 │ By: SI Suresh                       │   │
│  │ Source: Sharma Stores CCTV                                          │   │
│  │ Status: 📁 In Evidence Vault │ Size: 2.3 GB                        │   │
│  │ Chain: Collected → Hashed → Stored                                  │   │
│  │                                                                      │   │
│  │ Integrity: ✓ Verified │ Hash: sha256:d4e5f6...                     │   │
│  │ AI Analysis: Face detected, enhancement possible                    │   │
│  │                          [View] [Download] [Send for Enhancement]   │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ EVD-2026-KOR-00142-003 │ Physical │ Broken Lock                    │   │
│  │ ─────────────────────────────────────────────────────────────────── │   │
│  │ Collected: 02-Jan-2026 16:00 │ By: Const. Ramesh                   │   │
│  │ Location: Main gate of premises                                     │   │
│  │ Status: 🏢 Station Malkhana │ Locker: A-23                         │   │
│  │ Chain: Collected → Photographed → Sealed → Stored                  │   │
│  │                                                                      │   │
│  │ Integrity: ✓ Verified │ Seal No: KOR-2026-0456                     │   │
│  │                                         [View Photos] [Transfer]    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  + 5 more items [View All]                                                 │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  CHAIN OF CUSTODY LOG                                                       │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Select Evidence: [EVD-2026-KOR-00142-001 ▼]                        │   │
│  │                                                                      │   │
│  │ ┌─────────────────────────────────────────────────────────────────┐ │   │
│  │ │ 02-Jan 15:45 │ COLLECTED │ SI Suresh │ Crime Scene             │ │   │
│  │ │              │ GPS: 12.9352, 77.6245 │ Photo: Yes               │ │   │
│  │ │              │ Biometric: Verified                              │ │   │
│  │ ├─────────────────────────────────────────────────────────────────┤ │   │
│  │ │ 02-Jan 16:30 │ SEALED    │ SI Suresh │ PS Koramangala          │ │   │
│  │ │              │ Seal No: KOR-2026-0455                           │ │   │
│  │ │              │ Witness: HC Mohan                                │ │   │
│  │ ├─────────────────────────────────────────────────────────────────┤ │   │
│  │ │ 03-Jan 09:00 │ TRANSFERRED │ HC Mohan → FSL Courier           │ │   │
│  │ │              │ Receipt No: FSL-BLR-2026-00234                   │ │   │
│  │ │              │ Biometric: Both parties verified                 │ │   │
│  │ ├─────────────────────────────────────────────────────────────────┤ │   │
│  │ │ 03-Jan 14:00 │ RECEIVED   │ FSL Bangalore                      │ │   │
│  │ │              │ Lab Ref: FSL-BLR-FP-2026-0089                    │ │   │
│  │ │              │ Status: In Queue                                 │ │   │
│  │ └─────────────────────────────────────────────────────────────────┘ │   │
│  │                                                                      │   │
│  │ [Export Chain Report]  [Print for Court]  [Verify Integrity]        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  FORENSIC REQUESTS                                                          │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌───────────────┬────────────────┬─────────────┬───────────┬───────────┐ │
│  │ Request ID    │ Evidence       │ Test Type   │ Status    │ ETA       │ │
│  ├───────────────┼────────────────┼─────────────┼───────────┼───────────┤ │
│  │ FSL-0089      │ EVD-001        │ Fingerprint │ In Lab    │ 10-Jan    │ │
│  │ FSL-0090      │ EVD-002        │ Video Enh.  │ Pending   │ 15-Jan    │ │
│  └───────────────┴────────────────┴─────────────┴───────────┴───────────┘ │
│                                                                             │
│  [New Forensic Request]                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.9 Armoury & Inventory Screen

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ARMOURY MANAGEMENT │ PS: Koramangala │ Armoury In-Charge: HC Mohan        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──── INVENTORY STATUS ─────┐  ┌──── ISSUANCE TODAY ────┐                │
│  │                           │  │                         │                │
│  │ Pistols:    45/50        │  │ Issued:     12          │                │
│  │ Rifles:     23/25        │  │ Returned:   8           │                │
│  │ Ammunition: 2,340 rds    │  │ Pending:    4           │                │
│  │                           │  │                         │                │
│  │ Last Audit: 01-Jan-2026  │  │ Overdue:    2 ⚠️        │                │
│  │ Next Audit: 01-Apr-2026  │  │                         │                │
│  └───────────────────────────┘  └─────────────────────────┘                │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [🔫 Issue Weapon] [📥 Return Weapon] [📦 Ammunition] [📋 Audit] [📊 Report]│
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  WEAPONS REGISTRY                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Filter: [All Types ▼] [All Status ▼]          Search: [_______________]  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ID          │ Type       │ Make/Model    │ Status   │ Assigned To  │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ WPN-KOR-001 │ 9mm Pistol │ Glock 17      │ Issued   │ SI Suresh    │   │
│  │ WPN-KOR-002 │ 9mm Pistol │ Glock 17      │ Issued   │ ASI Prakash  │   │
│  │ WPN-KOR-003 │ 9mm Pistol │ Glock 17      │ In Armry │ -            │   │
│  │ WPN-KOR-004 │ .303 Rifle │ INSAS         │ Issued   │ Const. Kumar │   │
│  │ WPN-KOR-005 │ 9mm Pistol │ Glock 17      │ Maint.   │ Under Repair │   │
│  │ + 65 more   │            │               │          │ [View All]   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  AMMUNITION STOCK                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Type         │ In Stock   │ Issued (MTD) │ Min Level │ Status      │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ 9mm          │ 1,800      │ 450          │ 500       │ ✓ OK        │   │
│  │ .303         │ 340        │ 60           │ 200       │ ✓ OK        │   │
│  │ 7.62mm       │ 180        │ 20           │ 200       │ ⚠️ Low      │   │
│  │ Tear Gas     │ 20         │ 0            │ 10        │ ✓ OK        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  PENDING RETURNS (Overdue)                                                  │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⚠️ WPN-KOR-012 │ 9mm Pistol │ Const. Ravi │ Due: 03-Jan │ 1 day     │   │
│  │    Reason: Extended duty │ SHO Notified: Yes                        │   │
│  │                                              [Send Reminder] [Extend]│   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ ⚠️ WPN-KOR-018 │ .303 Rifle │ HC Shankar │ Due: 02-Jan │ 2 days    │   │
│  │    Reason: Bandobast duty │ SHO Notified: Yes                       │   │
│  │                                              [Send Reminder] [Extend]│   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  ISSUE WEAPON                                                               │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Officer:     [Select Officer ▼] (Biometric Required)                      │
│  Weapon:      [Select from Available ▼]                                    │
│  Ammunition:  [___] rounds                                                 │
│  Purpose:     [Duty ▼]                                                     │
│  Expected Return: [Date Picker]                                            │
│                                                                             │
│  [Issue - Requires Biometric]                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.10 Vehicle Allocation Screen

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ VEHICLE MANAGEMENT │ PS: Koramangala │ Transport In-Charge: ASI Prakash   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──── FLEET STATUS ─────────┐  ┌──── LIVE TRACKING ────────────────────┐ │
│  │                           │  │                                        │ │
│  │ Total Vehicles:  8        │  │  ┌────────────────────────────────┐   │ │
│  │ Active/On Road:  4        │  │  │     [MAP VIEW]                 │   │ │
│  │ Available:       2        │  │  │                                │   │ │
│  │ Under Maint:     1        │  │  │  🚔 KA-01-P-1234 - Patrol     │   │ │
│  │ Reserved:        1        │  │  │  🚔 KA-01-P-1235 - Patrol     │   │ │
│  │                           │  │  │  🚗 KA-01-G-5678 - Transport  │   │ │
│  │ Fuel Today: ₹12,450      │  │  │  🚐 KA-01-P-9999 - PCR        │   │ │
│  └───────────────────────────┘  │  │                                │   │ │
│                                  │  └────────────────────────────────┘   │ │
│                                  │  [Full Screen] [Historical] [Alerts]  │ │
│                                  └───────────────────────────────────────┘ │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  [🚗 Allocate] [📥 Return] [⛽ Fuel Log] [🔧 Maintenance] [📊 Reports]     │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  VEHICLE REGISTRY                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Reg No       │ Type      │ Status   │ Driver      │ Km Today │ Fuel │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ KA-01-P-1234 │ Patrol    │ On Duty  │ HC Mohan    │ 45 km    │ 78%  │   │
│  │ KA-01-P-1235 │ Patrol    │ On Duty  │ Const. Ravi │ 32 km    │ 65%  │   │
│  │ KA-01-G-5678 │ Gypsy     │ On Duty  │ Const. Kumar│ 28 km    │ 82%  │   │
│  │ KA-01-P-9999 │ PCR       │ On Duty  │ ASI Sharma  │ 67 km    │ 45%  │   │
│  │ KA-01-P-1236 │ Patrol    │ Available│ -           │ -        │ 90%  │   │
│  │ KA-01-G-5679 │ Gypsy     │ Available│ -           │ -        │ 95%  │   │
│  │ KA-01-P-1237 │ Patrol    │ Maint.   │ -           │ -        │ -    │   │
│  │ KA-01-B-0001 │ Bus       │ Reserved │ -           │ -        │ 100% │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  TODAY'S TRIPS                                                              │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Trip ID  │ Vehicle      │ Driver     │ Purpose      │ Km   │ Status │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ T-001    │ KA-01-P-1234 │ HC Mohan   │ Patrol Beat A│ 45   │ Active │   │
│  │ T-002    │ KA-01-G-5678 │ Const.Kumar│ Court Escort │ 28   │ Returned│  │
│  │ T-003    │ KA-01-P-9999 │ ASI Sharma │ PCR Duty     │ 67   │ Active │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│  ALLOCATE VEHICLE                                                           │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Vehicle:  [KA-01-P-1236 (Patrol - Available) ▼]                           │
│  Driver:   [Select Driver ▼] (Must have valid license)                     │
│  Purpose:  [Patrol ▼]                                                      │
│  Duration: From [__:__] To [__:__]                                         │
│  Remarks:  [________________________________]                              │
│                                                                             │
│  [Allocate]                                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Component Hierarchy (React/Next.js)

### 4.1 Application Structure

```
src/
├── app/                          # Next.js App Router
│   ├── (auth)/                   # Auth group (login, etc.)
│   │   ├── login/
│   │   │   └── page.tsx
│   │   └── layout.tsx
│   ├── (dashboard)/              # Protected routes
│   │   ├── layout.tsx            # Main dashboard layout
│   │   ├── page.tsx              # Dashboard home
│   │   ├── fir/
│   │   │   ├── page.tsx          # FIR list
│   │   │   ├── new/
│   │   │   │   └── page.tsx      # New FIR
│   │   │   └── [id]/
│   │   │       └── page.tsx      # FIR detail
│   │   ├── cases/
│   │   ├── evidence/
│   │   ├── personnel/
│   │   ├── armoury/
│   │   ├── vehicles/
│   │   ├── intelligence/         # State/Central only
│   │   ├── analytics/
│   │   └── settings/
│   └── api/                      # API routes (BFF)
│       ├── fir/
│       ├── cases/
│       └── sync/
│
├── components/
│   ├── ui/                       # Base UI components
│   │   ├── Button.tsx
│   │   ├── Input.tsx
│   │   ├── Select.tsx
│   │   ├── Modal.tsx
│   │   ├── Table.tsx
│   │   ├── Card.tsx
│   │   ├── Badge.tsx
│   │   ├── Alert.tsx
│   │   ├── Toast.tsx
│   │   ├── Tabs.tsx
│   │   ├── Tooltip.tsx
│   │   └── Skeleton.tsx
│   │
│   ├── layout/                   # Layout components
│   │   ├── Header.tsx
│   │   ├── Sidebar.tsx
│   │   ├── Footer.tsx
│   │   ├── Breadcrumbs.tsx
│   │   └── SyncStatusBar.tsx
│   │
│   ├── domain/                   # Domain-specific components
│   │   ├── fir/
│   │   │   ├── FIRForm.tsx
│   │   │   ├── FIRList.tsx
│   │   │   ├── FIRCard.tsx
│   │   │   ├── FIRStatusBadge.tsx
│   │   │   ├── VoiceInput.tsx
│   │   │   ├── HandwritingScanner.tsx
│   │   │   └── IPCSectionSelector.tsx
│   │   │
│   │   ├── cases/
│   │   │   ├── CaseDiary.tsx
│   │   │   ├── CaseTimeline.tsx
│   │   │   ├── CasePersons.tsx
│   │   │   └── SimilarCases.tsx
│   │   │
│   │   ├── evidence/
│   │   │   ├── EvidenceRegistry.tsx
│   │   │   ├── ChainOfCustody.tsx
│   │   │   ├── EvidenceUpload.tsx
│   │   │   └── ForensicRequest.tsx
│   │   │
│   │   ├── personnel/
│   │   │   ├── OfficerCard.tsx
│   │   │   ├── DutyRoster.tsx
│   │   │   ├── AttendanceLog.tsx
│   │   │   └── PerformanceMetrics.tsx
│   │   │
│   │   ├── armoury/
│   │   │   ├── WeaponRegistry.tsx
│   │   │   ├── AmmunitionStock.tsx
│   │   │   ├── IssuanceForm.tsx
│   │   │   └── AuditLog.tsx
│   │   │
│   │   ├── vehicles/
│   │   │   ├── FleetStatus.tsx
│   │   │   ├── VehicleMap.tsx
│   │   │   ├── TripLog.tsx
│   │   │   └── FuelLog.tsx
│   │   │
│   │   ├── intelligence/
│   │   │   ├── CrimeHeatmap.tsx
│   │   │   ├── NetworkGraph.tsx
│   │   │   ├── TrendAnalysis.tsx
│   │   │   └── AlertsPanel.tsx
│   │   │
│   │   └── command/
│   │       ├── StationStatus.tsx
│   │       ├── DistrictOverview.tsx
│   │       ├── StateMap.tsx
│   │       ├── NationalDashboard.tsx
│   │       └── AlertBroadcast.tsx
│   │
│   └── shared/                   # Shared/utility components
│       ├── BiometricAuth.tsx
│       ├── AuditTrail.tsx
│       ├── OfflineIndicator.tsx
│       ├── SyncProgress.tsx
│       ├── RoleGuard.tsx
│       ├── ConfirmDialog.tsx
│       └── ErrorBoundary.tsx
│
├── hooks/                        # Custom React hooks
│   ├── useAuth.ts
│   ├── useOffline.ts
│   ├── useSync.ts
│   ├── useBiometric.ts
│   ├── useAudit.ts
│   ├── useRole.ts
│   ├── useLocalStorage.ts
│   └── useDebounce.ts
│
├── stores/                       # State management (Zustand)
│   ├── authStore.ts
│   ├── syncStore.ts
│   ├── firStore.ts
│   ├── caseStore.ts
│   ├── alertStore.ts
│   └── uiStore.ts
│
├── lib/                          # Utilities
│   ├── api/
│   │   ├── client.ts             # API client
│   │   ├── offline.ts            # Offline queue
│   │   └── sync.ts               # Sync logic
│   ├── db/
│   │   └── indexedDB.ts          # Local storage
│   ├── crypto/
│   │   └── encrypt.ts            # Client-side encryption
│   └── utils/
│       ├── date.ts
│       ├── format.ts
│       └── validation.ts
│
├── types/                        # TypeScript types
│   ├── fir.ts
│   ├── case.ts
│   ├── evidence.ts
│   ├── personnel.ts
│   ├── auth.ts
│   └── api.ts
│
└── styles/
    ├── globals.css
    └── themes/
        ├── dark.css
        └── high-contrast.css
```

### 4.2 Component Hierarchy Diagram

```
App
├── Providers (Auth, Theme, Sync, Store)
│   └── Layout
│       ├── Header
│       │   ├── Logo
│       │   ├── UserInfo (Role, Name, Jurisdiction)
│       │   ├── SyncStatus
│       │   ├── NotificationBell
│       │   └── QuickActions
│       │
│       ├── Sidebar (Role-Aware)
│       │   ├── NavigationMenu
│       │   │   ├── NavItem (Dashboard)
│       │   │   ├── NavItem (FIR)
│       │   │   ├── NavItem (Cases)
│       │   │   ├── NavItem (Evidence)
│       │   │   ├── NavItem (Personnel) [SHO+]
│       │   │   ├── NavItem (Armoury)
│       │   │   ├── NavItem (Vehicles)
│       │   │   ├── NavItem (Intelligence) [District+]
│       │   │   ├── NavItem (Analytics) [District+]
│       │   │   └── NavItem (Settings)
│       │   └── StationSelector [District+]
│       │
│       ├── MainContent
│       │   ├── Breadcrumbs
│       │   ├── PageHeader
│       │   └── PageContent (Route-specific)
│       │
│       └── Footer
│           ├── SyncDetails
│           ├── OfflineIndicator
│           └── AuditInfo
│
└── Modals/Overlays
    ├── ConfirmDialog
    ├── BiometricPrompt
    ├── AlertPopup
    └── Toast
```

---

## 5. UI State Management Strategy

### 5.1 State Categories

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        STATE MANAGEMENT ARCHITECTURE                         │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ SERVER STATE (TanStack Query)                                               │
│ ─────────────────────────────                                               │
│ • FIR data                        • Background refetch                      │
│ • Case records                    • Stale-while-revalidate                 │
│ • Personnel data                  • Optimistic updates                      │
│ • Evidence records                • Offline persistence                     │
│ • Analytics data                  • Query invalidation                      │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ CLIENT STATE (Zustand)                                                      │
│ ─────────────────────                                                       │
│ • Auth state                      • Middleware support                      │
│ • Sync status                     • DevTools integration                    │
│ • UI preferences                  • Persist to localStorage                 │
│ • Active alerts                   • Computed values                         │
│ • Draft forms                     • Actions                                 │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ OFFLINE STATE (IndexedDB via Dexie)                                         │
│ ──────────────────────────────────                                          │
│ • Pending sync queue              • Full CRUD support                       │
│ • Cached server data              • Indexed queries                         │
│ • Draft documents                 • Blob storage                            │
│ • Evidence files                  • Sync metadata                           │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ URL STATE (Next.js)                                                         │
│ ───────────────────                                                         │
│ • Current page                    • Search params                           │
│ • Filters                         • Modal state                             │
│ • Sort order                      • Tab selection                           │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Zustand Store Structure

```typescript
// stores/authStore.ts
interface AuthState {
  user: User | null;
  role: Role | null;
  jurisdiction: Jurisdiction | null;
  token: string | null;
  isAuthenticated: boolean;
  biometricVerified: boolean;
  lastActivity: Date;

  // Actions
  login: (credentials: Credentials) => Promise<void>;
  logout: () => void;
  refreshToken: () => Promise<void>;
  verifyBiometric: () => Promise<boolean>;
}

// stores/syncStore.ts
interface SyncState {
  isOnline: boolean;
  lastSyncTime: Date | null;
  pendingCount: number;
  syncStatus: 'idle' | 'syncing' | 'error';
  syncProgress: number;

  // Actions
  checkConnectivity: () => Promise<boolean>;
  triggerSync: () => Promise<void>;
  queueForSync: (item: SyncItem) => void;
  getSyncQueue: () => SyncItem[];
}

// stores/alertStore.ts
interface AlertState {
  activeAlerts: Alert[];
  unreadCount: number;
  criticalAlerts: Alert[];

  // Actions
  addAlert: (alert: Alert) => void;
  dismissAlert: (id: string) => void;
  acknowledgeAlert: (id: string) => void;
  markAllRead: () => void;
}

// stores/uiStore.ts
interface UIState {
  sidebarCollapsed: boolean;
  theme: 'dark' | 'light' | 'high-contrast';
  fontSize: 'normal' | 'large';
  reducedMotion: boolean;
  currentModal: string | null;

  // Actions
  toggleSidebar: () => void;
  setTheme: (theme: Theme) => void;
  openModal: (modalId: string) => void;
  closeModal: () => void;
}
```

### 5.3 Offline-First Data Flow

```
USER ACTION
     │
     ▼
┌─────────────────────────────────────┐
│ 1. OPTIMISTIC UPDATE                │
│    • UI updated immediately         │
│    • Zustand state modified         │
│    • User sees success              │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 2. PERSIST TO INDEXEDDB             │
│    • Save to local database         │
│    • Include sync metadata          │
│    • Mark as 'pending_sync'         │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 3. QUEUE FOR SYNC                   │
│    • Add to sync queue              │
│    • Assign priority                │
│    • Set retry policy               │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 4. ATTEMPT SERVER SYNC              │
│    IF ONLINE:                       │
│      • Send to server               │
│      • Wait for confirmation        │
│      • Mark as 'synced'             │
│    IF OFFLINE:                      │
│      • Keep in queue                │
│      • Show pending indicator       │
│      • Retry when online            │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 5. HANDLE RESPONSE                  │
│    SUCCESS:                         │
│      • Update IndexedDB             │
│      • Invalidate TanStack Query    │
│      • Remove from sync queue       │
│    FAILURE:                         │
│      • Mark for retry               │
│      • Show error notification      │
│      • Keep local changes           │
│    CONFLICT:                        │
│      • Show conflict resolution UI  │
│      • Let user choose version      │
└─────────────────────────────────────┘
```

### 5.4 Role-Based UI Rendering

```typescript
// components/shared/RoleGuard.tsx
interface RoleGuardProps {
  allowedRoles: Role[];
  minimumTier?: Tier;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}

// Usage example
<RoleGuard
  allowedRoles={['SHO', 'SP', 'DGP']}
  minimumTier="DISTRICT"
  fallback={<AccessDenied />}
>
  <IntelligenceDashboard />
</RoleGuard>

// Role-based navigation config
const navigationConfig: NavConfig = {
  dashboard: { roles: ['ALL'], tiers: ['ALL'] },
  fir: { roles: ['ALL'], tiers: ['ALL'] },
  cases: { roles: ['ALL'], tiers: ['ALL'] },
  evidence: { roles: ['ALL'], tiers: ['ALL'] },
  personnel: {
    roles: ['SHO', 'SP', 'DGP', 'ADMIN'],
    tiers: ['STATION', 'DISTRICT', 'STATE', 'CENTRAL']
  },
  armoury: { roles: ['ALL'], tiers: ['ALL'] },
  vehicles: { roles: ['ALL'], tiers: ['ALL'] },
  intelligence: {
    roles: ['SP', 'DGP', 'ANALYST'],
    tiers: ['DISTRICT', 'STATE', 'CENTRAL']
  },
  analytics: {
    roles: ['SHO', 'SP', 'DGP', 'ANALYST'],
    tiers: ['STATION', 'DISTRICT', 'STATE', 'CENTRAL']
  },
  command: {
    roles: ['DGP', 'SECRETARY'],
    tiers: ['STATE', 'CENTRAL']
  },
  settings: { roles: ['ADMIN'], tiers: ['ALL'] },
};
```

---

## 6. Critical UI Behaviors

### 6.1 Offline Indicators

```
ONLINE STATE:
┌─────────────────────────────────────────────────────────────────┐
│ ● Online │ Last sync: 2 min ago │ All changes synced           │
└─────────────────────────────────────────────────────────────────┘

OFFLINE STATE:
┌─────────────────────────────────────────────────────────────────┐
│ ○ Offline │ 5 changes pending │ Will sync when connected       │
│ [Working locally - all features available]                     │
└─────────────────────────────────────────────────────────────────┘

SYNCING STATE:
┌─────────────────────────────────────────────────────────────────┐
│ ◐ Syncing... │ 3/5 items │ ████████░░ 60%                      │
└─────────────────────────────────────────────────────────────────┘

ERROR STATE:
┌─────────────────────────────────────────────────────────────────┐
│ ⚠ Sync Error │ 2 items failed │ [Retry] [View Details]         │
└─────────────────────────────────────────────────────────────────┘
```

### 6.2 Audit Trail Display

```
Every screen shows:
┌─────────────────────────────────────────────────────────────────┐
│ Viewing as: SI Suresh (KAR-SI-4567) │ PS: Koramangala          │
│ Session: 14:32 - Active │ All actions are being logged         │
└─────────────────────────────────────────────────────────────────┘
```

### 6.3 AI Advisory Disclaimers

```
Every AI suggestion shows:
┌─────────────────────────────────────────────────────────────────┐
│ 🤖 AI SUGGESTION                                                │
│ ───────────────                                                 │
│ [Content of AI suggestion]                                     │
│                                                                 │
│ ⚠️ This is AI-generated analysis for advisory purposes only.   │
│ Human verification required before any action.                 │
│ Confidence: 78% │ Model: NPDMS-NLP-v2.1                        │
│                                                                 │
│ [Accept] [Modify] [Reject] [View Reasoning]                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Appendix: Keyboard Shortcuts

| Action | Shortcut | Context |
|--------|----------|---------|
| New FIR | Ctrl/Cmd + N | Global |
| Search | Ctrl/Cmd + K | Global |
| Save | Ctrl/Cmd + S | Forms |
| Quick Actions | Ctrl/Cmd + / | Global |
| Toggle Sidebar | Ctrl/Cmd + B | Global |
| Previous Page | Alt + ← | Navigation |
| Next Page | Alt + → | Navigation |
| Close Modal | Esc | Modals |
| Emergency Alert | Ctrl/Cmd + E | Global (SHO+) |
| Help | F1 | Global |
