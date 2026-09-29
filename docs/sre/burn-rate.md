# Burn rate

Burn rate is `error_ratio / (1 - objective)`.

A burn rate of 1 consumes the window's budget exactly at the end of the window. The fast-burn defaults are 14.4 on one- and five-minute windows and 6 on thirty-minute and one-hour windows. The compliance window alerts at 1, which is the breach itself.

`slo.MultiBurn` pages only when the short and long windows both exceed their thresholds. A one-minute spike with a quiet hour does not page. The engine records per-window status; the multi-window helper is available to alert routing and is covered by tests.
