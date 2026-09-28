# Error budget

For an objective of 99.95%, the allowed error ratio is 0.05%. If 28 of 10,000 requests fail, the error ratio is 0.28% and the burn rate is 5.6. Budget remaining for that window is `(1 - 5.6) * 100`, which is negative. The status is `breached` because the SLI is below the objective.

The illustrative JSON in the product brief used round numbers that do not match this formula. The implementation uses the formula, and the tests lock it in. Results are not constants in the API.
