# ADR-008 SLO calculation strategy

Status: accepted

Callers supply good and total counts. The SLI is good/total. Burn rate is the error ratio divided by `1 - objective`. Budget remaining is `1 - burn rate`, as a percentage of that window, and may be negative. A window with no requests is `no_data`. A window below the objective is `breached`. A window still meeting the objective but above its burn alert is `at_risk`. Multi-window paging requires both windows to exceed their thresholds. Objectives of 100% are rejected because they have no budget.
