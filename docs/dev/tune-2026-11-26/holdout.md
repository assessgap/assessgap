2025 holdout: 218492 appeals scored by the current rule with 3+ comps (68092 pro se), 846 neighborhoods; bootstrap 2000 reps by neighborhood

**AUC, pro se (n 68092)**

| method | AUC | 95% CI | Δ vs current | 95% CI of Δ |
|---|--:|--:|--:|--:|
| current | 0.7889 | 0.7831 – 0.7943 | | |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 0.8166 | 0.8109 – 0.8220 | +0.0277 | +0.0250 – +0.0304 |
| F2_stat_pctile_gap | 0.7985 | 0.7928 – 0.8037 | +0.0096 | +0.0078 – +0.0115 |
| F3_logistic_all | 0.8016 | 0.7963 – 0.8067 | +0.0128 | +0.0104 – +0.0152 |

**AUC, attorney (n 150400)**

| method | AUC | 95% CI | Δ vs current | 95% CI of Δ |
|---|--:|--:|--:|--:|
| current | 0.7514 | 0.7453 – 0.7579 | | |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 0.7773 | 0.7712 – 0.7837 | +0.0259 | +0.0233 – +0.0284 |
| F2_stat_pctile_gap | 0.7630 | 0.7573 – 0.7688 | +0.0116 | +0.0093 – +0.0140 |
| F3_logistic_all | 0.7696 | 0.7639 – 0.7754 | +0.0182 | +0.0157 – +0.0208 |

**AUC, all (n 218492)**

| method | AUC | 95% CI | Δ vs current | 95% CI of Δ |
|---|--:|--:|--:|--:|
| current | 0.7615 | 0.7565 – 0.7668 | | |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 0.7879 | 0.7827 – 0.7933 | +0.0264 | +0.0241 – +0.0285 |
| F2_stat_pctile_gap | 0.7722 | 0.7674 – 0.7772 | +0.0107 | +0.0089 – +0.0126 |
| F3_logistic_all | 0.7777 | 0.7730 – 0.7823 | +0.0162 | +0.0140 – +0.0185 |

**Other measures (pro se)**

| method | top 25% by score: win rate (share) | Spearman, score vs % cut among winners | tier calibration error (2023–24 rates quoted for 2025) |
|---|--:|--:|--:|
| current | 89.2% (25.0%) | 0.234 | 0.0260 |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 90.7% (25.0%) | 0.204 | 0.0294 |
| F2_stat_pctile_gap | 89.6% (25.0%) | 0.193 | 0.0303 |
| F3_logistic_all | 90.9% (25.0%) | 0.144 | - |

**Tier win rates, pro se: 2023–24 (tuning) and 2025, share of appeals / reduction rate**

| method | years | band 1 | band 2 | band 3 | band 4 |
|---|---|--:|--:|--:|--:|
| current | bands | >=+5% | +1..+5% | in line | < -1% |
| current | 2023–24 pro se | 21.6% / 89.5% | 16.6% / 83.3% | 14.5% / 63.5% | 47.2% / 34.5% |
| current | 2025 pro se | 19.4% / 89.2% | 17.5% / 85.7% | 15.0% / 65.8% | 48.2% / 38.2% |
| current | 2025 attorney | 17.2% / 77.5% | 16.2% / 75.0% | 16.1% / 51.7% | 50.5% / 29.0% |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | bands | >=75% lower | 50-75% | 25-50% | <25% |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 2023–24 pro se | 20.9% / 92.0% | 25.6% / 79.4% | 27.7% / 50.2% | 25.8% / 20.3% |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 2025 pro se | 20.4% / 91.3% | 24.9% / 80.9% | 27.3% / 54.9% | 27.4% / 24.5% |
| F1_w4_wsf1_bsmt0_k30_pctile_gap | 2025 attorney | 18.2% / 80.6% | 23.8% / 68.0% | 28.3% / 42.4% | 29.6% / 18.9% |
| F2_stat_pctile_gap | bands | >=75% lower | 50-75% | 25-50% | <25% |
| F2_stat_pctile_gap | 2023–24 pro se | 22.7% / 90.8% | 23.2% / 76.5% | 24.6% / 52.9% | 29.5% / 24.8% |
| F2_stat_pctile_gap | 2025 pro se | 22.1% / 90.2% | 22.3% / 78.8% | 24.1% / 57.0% | 31.4% / 29.2% |
| F2_stat_pctile_gap | 2025 attorney | 19.8% / 79.5% | 21.9% / 65.4% | 25.6% / 44.5% | 32.8% / 21.6% |
