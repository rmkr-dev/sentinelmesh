# Burn rate

Burn rate is `error_ratio / (1 - objective)`.

A burn rate of 1 consumes the window's budget exactly at the end of the window. The fast-burn defaults are 14.4 on one- and five-minute windows and 6 on thirty-minute and one-hour windows. The compliance window alerts at 1, which is the breach itself.

`slo.MultiBurn` pages only when the short and long windows both exceed their thresholds (14.4 and 6). The engine emits that page signal from `Tick` after it evaluates the windows. A one-minute spike with a quiet longer window does not page.
