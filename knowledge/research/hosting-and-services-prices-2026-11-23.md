# Hosting and service prices in Russia: November 2026 recheck

Checked 2026-11-23. Prices, quotas and billing rules are volatile; recheck them in each provider's calculator before ordering. This file is agent research: it refreshes the unit prices of the [hosting research](hosting-2026-10-22.md) and the [cost levers research](cost-levers-and-market-2026-10-25.md) and decides nothing. Each row says whether the figure changed since those October files.

## Method and conventions

- Sources are official provider pages and documentation unless marked **third-party**. Facts that no reachable official page confirmed are marked **unconfirmed**.
- A month is 720 hours (30 days), as in the October files; VK Cloud's monthly figures are per 30 days.
- Yandex Cloud's Russian price tables still render only in the browser ("Нет данных" without JavaScript). Ruble figures for Yandex Cloud are therefore derived from the USD rows for the Russia region (excluding VAT) in `docs.yandex.cloud/int/en/<service>/pricing.md` as RUB including VAT = USD × 122, the factor the hosting research established. The factor still reproduces the ruble figures the Russian compute page states directly (Ice Lake vCPU 1.24 ₽ an hour, RAM 0.33 ₽ per GB an hour, page updated 2026-11-13). Each such figure is **derived**.
- "Unchanged" means the same figure, effective date or price list date as in the October files.

## VAT

- Russia's general VAT rate is 22% for supplies made from 2026-01-01 (Federal Law of 2025-11-28 No. 425-FZ; the tax service's letter of 2025-12-29 No. СД-4-3/11802@ on the transition; [tax service news](https://www.nalog.gov.ru/rn26/news/smi/16596761/), [letter](https://www.nalog.gov.ru/rn27/taxation/taxes/nds/16594097/)). Confirmed.
- Provider statements: Selectel says all prices include 22% VAT ([prices](https://selectel.ru/prices/)); Yandex Cloud says ruble prices include VAT and USD prices exclude it, without naming the rate ([price list](https://yandex.cloud/ru/price-list), [compute pricing](https://yandex.cloud/ru/docs/compute/pricing)); VK Cloud's price list columns read "в том числе НДС" without naming the rate ([price list](https://cloud.vk.ru/pricelist)); Unisender Go says its prices are not subject to VAT ("НДС не облагается", [prices](https://go.unisender.ru/)); Timeweb Cloud's server page makes no VAT statement.
- Did prices move with VAT? Every provider move found happened before the October files and is already in their figures: VK Cloud issued a new price list dated 2026-01-12; Selectel raised most services by at most 8% on 2026-01-15 ([notice](https://files.selectel.ru/docs/ru/New-prices-January-15-2026.pdf)); Yandex Cloud indexed compute by 8–10%, Managed PostgreSQL by 5%, Object Storage and DNS by 8% from 2026-05-01, with Postbox not indexed and no VAT reason given ([announcement](https://yandex.cloud/ru/blog/pricing-update-2026)). No provider price change between 2026-10-22 and 2026-11-23 was found.
- Pressure ahead: Selectel's chief executive said server hardware delivered to Russia roughly doubled in price since October 2025, and analysts expect cloud tariffs to follow with a lag of three to six months ([ComNews, 2026-10-09](https://comnews.ru/content/247780/2026-10-09/2026-w41/1008/cena-servery-dlya-codov-vyrosla-dva-raza), **third-party**). No announced rise was found.

## 1. VK Cloud

Price list dated 2026-01-12, incl. VAT, per 30 days ([price list](https://cloud.vk.ru/pricelist)).

| Item | Price per month | Since October |
|---|---|---|
| Standard vCPU (Intel Cascade Lake) | 820 ₽ | unchanged |
| High-frequency vCPU | 1,297 ₽ | unchanged |
| RAM per GB | 223 ₽ | unchanged |
| SSD per GB | 13 ₽ (High-IOPS SSD 15 ₽; HDD 4 ₽ but "temporarily unavailable for order") | unchanged |
| Public IPv4 | 187 ₽ on a VM port, 188 ₽ floating | unchanged |
| Cloud Databases vCPU / RAM per GB / SSD per GB | 1,024.49 ₽ / 278.31 ₽ / 16.59 ₽ (the page does not say these rows apply to PostgreSQL specifically) | unchanged |
| Object Storage Hotbox / Icebox per GB | 2.28 ₽ / 1.09 ₽; egress 1.31 ₽ / 1.67 ₽ per GB | unchanged |
| VM internet traffic | not charged | unchanged ([traffic billing](https://cloud.vk.ru/docs/en/networks/vnet/tariffication)) |

**Stopped VMs** ([VM billing](https://cloud.vk.ru/docs/en/computing/iaas/tariffication)): "CPU (vCPU) and RAM are charged only when the virtual machine is running"; disks, public IP addresses, backups and paid licences keep being charged; billing is to the nearest minute. Unchanged.

## 2. Yandex Cloud (Russia region)

All rows below are effective 2026-04-30 (DNS query rows 2026-07-23, VPC rows 2025-12-31) with no end date, the same rows the October files used. No price notes dated after 2026-10-01 appear on the pricing pages.

| Item | USD excl. VAT | ₽ incl. VAT per month (derived) | Since October | Source |
|---|---|---|---|---|
| Ice Lake vCPU 100% | 0.0101639328 /h | 892.80 | unchanged | [compute pricing](https://docs.yandex.cloud/int/en/compute/pricing.md) |
| Ice Lake RAM per GB | 0.0027049176 /h | 237.60 | unchanged | same |
| Ice Lake vCPU 20% / 50% | 0.0042622944 / 0.00614754 /h | 374.40 / 540.00 | not in October files | same |
| Ice Lake compute-optimized (`highfreq-v3`) vCPU / RAM per GB | 0.0170491776 / 0.0031098356 /h | 1,497.60 / 273.17 | unchanged | same |
| AMD Zen 4 vCPU 100% / RAM per GB (the newest non-GPU platform listed; Sapphire Rapids is not listed) | 0.0103278672 / 0.002771311 /h | 907.20 / 243.43 | not in October files | same |
| AMD Zen 4 compute-optimized vCPU / RAM per GB | 0.0209836032 / 0.0038499994 /h | 1,843.20 / 338.18 | not in October files | same |
| Committed volume (CVoS) Ice Lake, 1 year: vCPU / RAM per GB | 0.008606556 / 0.0022950816 /h | 756.00 / 201.60 (about 15% off) | unchanged | same |
| CVoS Ice Lake, 6 months: vCPU / RAM per GB | 0.0091803264 / 0.0024491799 /h | 806.40 / 215.14 (about 10% off) | unchanged | same |
| Preemptible Ice Lake vCPU / RAM per GB | 0.0027868848 / 0.0006803278 /h | 244.80 / 59.76 | unchanged | same |
| `network-ssd` per GB | 0.0001631147 /GB-h | 14.33 | unchanged | same |
| `network-hdd` / `network-ssd-nonreplicated` per GB | 0.0000393443 / 0.0001204918 /GB-h | 3.46 / 10.58 | unchanged | same |
| Public IP, attached to a running VM | 0.0021599997 /h | 189.73 | unchanged | [VPC pricing](https://docs.yandex.cloud/int/en/vpc/pricing.md) |
| Static IP, unattached or on a stopped VM | +0.0027899996 /h (0.0049499993 /h in total) | 434.81 | not in October files | same |
| VM internet egress | first 100 GB free, then 0.0116393424 /GB | 1.42 per GB | unchanged | same |
| Managed PostgreSQL Ice Lake vCPU / RAM per GB | 0.0154032762 / 0.0041573764 /h | 1,353.02 / 365.18 | unchanged | [PostgreSQL pricing](https://docs.yandex.cloud/int/en/managed-postgresql/pricing.md) |
| Managed PostgreSQL `network-ssd` per GB; backup beyond the cluster's storage size | 0.0001786885 / 0.0000251639 /GB-h | 15.70; 2.21 | unchanged | same |
| Object Storage standard per GB (first 1 GB free) | 0.0000270492 /GB-h | 2.376 | unchanged | [storage pricing](https://docs.yandex.cloud/int/en/storage/pricing.md) |
| Object Storage cold / ice per GB (ice: 12-month minimum) | 0.0000144262 / 0.0000072131 /GB-h | 1.267 / 0.634 | unchanged | same |
| Object Storage requests, standard | PUT 0.004665573 per 1,000; GET 0.0037704912 per 10,000 (10,000 PUT and 100,000 GET free a month) | 0.569 / 0.46 | unchanged | same |
| Object Storage egress | 0–100 GB free; 100–1,024 GB 0.0137699978; 1,024–51,200 GB 0.0115199982 per GB | 1.68 / 1.41 per GB | unchanged | same |
| Postbox | first 2,000 emails a month free; 2,001–10,000 at 0.6583332279 and 10,001–50,000 at 0.574999908 per 1,000 | 80.32 / 70.15 per 1,000 | unchanged | [Postbox pricing](https://docs.yandex.cloud/int/en/postbox/pricing.md) |
| Cloud DNS | 0.0004852458 per zone-hour; 0.3109835568 per million authoritative queries | 42.62 per zone; 37.94 per million | unchanged | [DNS pricing](https://docs.yandex.cloud/int/en/dns/pricing.md) |

- **Smallest production Managed PostgreSQL:** the pricing example still uses `s3-c2-m8` (2 vCPU 100%, 8 GB, Ice Lake); the October file found no 2 vCPU / 4 GB class at 100% and burstable classes closed to new clusters.
- **Postbox quotas** (adjustable through support or the quota manager): 1 email a second, 200 emails per 24 hours, 200 recipients per email, 30 MB per email ([limits](https://docs.yandex.cloud/int/en/postbox/concepts/limits.md)). Unchanged. Every email accepted for sending is charged, and an email to several recipients counts once per recipient.
- **Stopped VMs:** compute is not charged while stopped, disks are ([pricing policy](https://yandex.cloud/en/docs/compute/pricing)); a static IP on a stopped VM costs the higher inactive rate above.
- **October 2026 incident** (new since the October files): Yandex Cloud ran in a limited mode after damage to its data centres and made all services free from 2026-10-08 to 2026-10-31 for customers active on 2026-10-07; billing resumed on 2026-11-01 ([offer](https://yandex.cloud/ru/offer-october)). On 2026-11-10 it reported restoring 97.16% of customer data, said the remaining 2.84% cannot be recovered and that customers using its backup tools lost nothing, enabled backups by default for new VMs and opened a new zone `ru-central1-f` ([Anti-Malware](https://www.anti-malware.ru/news/2026-11-10-111332/51998), [Interfax](https://interfax.com/newsroom/top-stories/119857/), **third-party**). The cost of default VM backups (Cloud Backup was ≈281 ₽ per VM plus ≈4.97 ₽ per GB in October) was not rechecked; whether the default can be turned off is **unconfirmed**.

## 3. Selectel

| Item | Price | Since October | Source |
|---|---|---|---|
| VAT | all prices include 22% VAT | unchanged | [prices](https://selectel.ru/prices/) |
| Cloud Standard Line | from 948.50 ₽ a month (configuration behind "from" **unconfirmed**) | unchanged | [cloud servers](https://selectel.ru/services/cloud/servers/) |
| Cloud HighFreq Line (Xeon Gold 6354, EPYC 9474F) | from 1,809.79 ₽ a month | not in October files | same |
| Cloud unit prices (vCPU, RAM, disk, IP) | **unconfirmed**: the price page showed "Актуализация цен..." ("prices are being updated") in every cloud and dedicated table | not rechecked | [prices](https://selectel.ru/prices/) |
| Cloud billing | hourly; stopped or paused servers keep being billed in full; only "frozen" servers stop paying for vCPU, RAM and GPU; 3 TB of external traffic free per account a month | not in October files (billing) | [payment rules](https://docs.selectel.ru/cloud-servers/about/payment/) |
| Dedicated server, 8 cores / 32 GB | AR31-NVMe: AMD Ryzen 7 3700X (8 cores, 3.6 GHz), 32 GB DDR4 non-ECC, 2 × 500 GB NVMe, 11,000 ₽ a month (**third-party**, review dated 2026-11-23; the official list did not load, so **unconfirmed**); a Ryzen 7 7700X / 64 GB DDR5 configuration (AR42-NVMe) at 18,000 ₽ (**third-party**) | October: ≈11,000 ₽ old snapshot, estimated ≈12,000 ₽ after the January rise; still **unconfirmed** | [free-hosting.ru review](https://free-hosting.ru/selectel/), **third-party**; [dedicated](https://selectel.ru/services/dedicated/) |
| Dedicated terms | 1 Gbit/s port with unlimited traffic; ready servers in about 2 minutes without an OS, 7–60 minutes with one; custom builds 1–5 days | unchanged | [dedicated](https://selectel.ru/services/dedicated/) |
| DNS hosting | free | unchanged | [prices](https://selectel.ru/prices/) |
| S3 Vault (cold) | 1.43 ₽ per GB-month, egress 1.96 ₽ per GB | not in October files | [prices](https://selectel.ru/prices/) |

## 4. Timeweb Cloud (fixed VPS with hourly billing, unmetered traffic)

Official server page, Moscow, read in a browser on 2026-11-23 ([cloud servers](https://timeweb.cloud/services/cloud-servers)). The page states hourly billing ("с почасовой оплатой"), no traffic limit and a 1 Gbit/s port per tariff. Prices include the first public IPv4; turning it off lowers a tariff by 200 ₽ (observed on MSK 80: 7,000 → 6,800 ₽). VAT is not stated.

| Line and tariff | vCPU / RAM / NVMe | Price per month | Since October |
|---|---|---|---|
| Premium 3.3 GHz, MSK 15 | 1 / 1 GB / 15 GB | 1,380 ₽ | not in October files |
| Premium, MSK 50 | 2 / 4 GB / 50 GB | 3,840 ₽ | October cited 1,080 ₽ (**third-party**); not comparable, see below |
| Premium, MSK 80 | 4 / 8 GB / 80 GB | 7,000 ₽ | October cited 1,800 ₽ (**third-party**); not comparable |
| Premium, MSK 160 | 8 / 16 GB / 160 GB | 13,800 ₽ | not in October files |
| Ultra 3.3 GHz, MSK Ultra 50 | 2 / 4 GB / 50 GB | 5,480 ₽ | unchanged (official) |
| Dedicated CPU 3.3 GHz, Dedic CPU-100 | 4 / 16 GB / 100 GB | 16,200 ₽ | not in October files |
| Dedicated CPU, Dedic CPU-120 | 8 / 32 GB / 120 GB | 30,280 ₽ | not in October files |

The October Premium figures came from a third-party list; the official Premium prices today are about 3.6–3.9 times higher. The official Ultra price is unchanged, so whether Premium was repriced or the third-party list was wrong is **unconfirmed**; no Timeweb Cloud server price announcement was found (Timeweb raised some shared web hosting tariffs from 2026-09-29, [news](https://timeweb.com/news/izmenenie-stoimosti-tarifov-virtualnogo-hostinga/)). Whether shared lines guarantee full cores is not stated.

**Timeweb dedicated servers** (official, [dedicated](https://timeweb.cloud/services/dedicated-server)), 1 Gbit/s included: Intel Core i5-12600 (6 cores, 12 threads), 32 GB, 2 × 1 TB NVMe, 9,570 ₽; Intel Core i9-10900K (10 cores, 20 threads), 64 GB, 2 × 1 TB NVMe, 15,900 ₽; Intel Xeon E-2288G (8 cores, 16 threads), 64 GB, 2 × 960 GB SSD, 18,580 ₽. October saw only a 6-core / 32 GB / 2 × 256 GB SSD server at 7,250 ₽, a different configuration.

## 5. Unisender Go

Card plans, not subject to VAT ([prices](https://go.unisender.ru/)). All unchanged since October.

| Plan (emails a month) | Price | Extra per 1,000 |
|---|---|---|
| 6,000 | 800 ₽; free for new users for the first 2 months; 500 emails a day until support raises it | 150 ₽ |
| 10,000 | 1,200 ₽ | 140 ₽ |
| 50,000 | 2,750 ₽ | 70 ₽ |
| 100,000 | 5,000 ₽ | 65 ₽ |

Prepaying 3, 6 or 12 months gives 5%, 10% or 20% off; invoice plans start at 100,000 emails for 6,000 ₽.

## 6. Domains and TLS

| Registrar | .ru registration | .ru renewal | Since October | Source |
|---|---|---|---|---|
| REG.RU | 169 ₽ first year (promotion on the zone page) | 1,199 ₽ from 2026-04-01 per REG.RU news seen only as a search summary (**unconfirmed**; an earlier notice gave 999 ₽ from 2026-01-01) | renewal was **unconfirmed** in October | [.ru page](https://www.reg.ru/domain/new/ru) |
| RU-CENTER | 747 ₽; transfer in 376 ₽ | "according to the tariff plan", figure **unconfirmed** | October cited 2,899 ₽ renewal from a search snippet (**unconfirmed**) | [.ru page](https://www.nic.ru/catalog/domains/ru/) |
| Timeweb | not stated | 399 ₽ on the standard renewal plan (Extra 499 ₽, Premium 899 ₽ with a 5-year fixed price) | unchanged | [renewal plans](https://timeweb.cloud/docs/domains/domain-renewal-plans) |

VAT treatment of domain prices is not stated on these pages.

**Let's Encrypt and .ru:** certificates for ordinary .ru domains are still issued, with a sanctions screen. The subscriber agreement version 1.7 of 2026-06-04 added a requirement that subscribers are not subject to comprehensive US sanctions or export controls and do not act for sanctioned parties; a Let's Encrypt representative said private individuals and non-government organisations in Russia can still obtain certificates, while government bodies are not served ([SecurityLab, 2026-06-10](https://www.securitylab.ru/news/573620.php), **third-party**). Let's Encrypt community staff say .ru domains were never blocked for the zone alone, and individual domains of sanctioned owners are refused with "forbidden by policy" ([community forum](https://community.letsencrypt.org/t/russian-domains-not-blocked-anymore/188429), **third-party**). The current agreement is version 1.8 of 2026-07-06 ([repository](https://letsencrypt.org/repository/)); its sanctions text was not read (**unconfirmed**). Other certificate authorities restricted Russian customers in 2026 (GlobalSign revocations from 2026-06-13 were reported, **third-party**), so a fallback such as the Russian national certificate authority would be a separate decision. Certificate lifetimes still fall to 64 days on 2027-02-10 ([announcement](https://letsencrypt.org/2025/12/02/from-90-to-45/)).

## 7. TURN relay VM

A TURN relay forwards media, so unmetered traffic matters more than the CPU price.

| Provider | Configuration | Monthly price incl. VAT | Traffic |
|---|---|---|---|
| VK Cloud | 1 standard vCPU, 1 GB, 10 GB SSD, IPv4 | 820 + 223 + 10 × 13 + 187 = **1,360 ₽** (availability of a 1 vCPU / 1 GB flavour **unconfirmed**) | not charged |
| Timeweb Cloud | Premium MSK 15: 1 vCPU, 1 GB, 15 GB NVMe, IPv4 included | **1,380 ₽** (VAT not stated) | unmetered |
| Yandex Cloud | 2 vCPU at 20%, 2 GB, 10 GB `network-hdd`, IPv4 | 374.4 × 2 + 237.6 × 2 + 34.56 + 189.73 ≈ 1,448 ₽ (derived) | ≈1.42 ₽ per GB after 100 GB |

## Monthly cost building blocks

₽ per month including VAT (Unisender Go: no VAT), from the prices above; 720 hours a month.

| Block | Arithmetic | Monthly |
|---|---|---|
| (a) App VM, Yandex Cloud Ice Lake 100%, 2 vCPU / 4 GB / 40 GB `network-ssd`, IPv4 | 2 × 892.80 + 4 × 237.60 + 40 × 14.33 + 189.73 = 1,785.60 + 950.40 + 573.12 + 189.73 | **≈3,499 ₽** (derived); with 1-year CVoS 2 × 756 + 4 × 201.6 + 573.12 + 189.73 ≈ 3,081 ₽ |
| (b) App VM, Yandex Cloud, 4 vCPU / 8 GB / 80 GB, IPv4 | 4 × 892.80 + 8 × 237.60 + 80 × 14.33 + 189.73 = 3,571.20 + 1,900.80 + 1,146.24 + 189.73 | **≈6,808 ₽** (derived); 1-year CVoS ≈5,973 ₽; Timeweb Premium MSK 80 (4 / 8 / 80, IPv4) 7,000 ₽ |
| (c) BBB host, VK Cloud standard, 4 vCPU / 8 GB / 50 GB SSD, IPv4, always on | 4 × 820 + 8 × 223 + 50 × 13 + 187 = 3,280 + 1,784 + 650 + 187 | **5,901 ₽** |
| (c) same, running 30% of hours | (3,280 + 1,784) × 0.30 + 650 + 187 = 1,519.20 + 837 | **≈2,356 ₽** |
| (c) same, running 62.5% of hours | 5,064 × 0.625 + 837 = 3,165 + 837 | **4,002 ₽** |
| (d) BBB host, VK Cloud standard, 8 vCPU / 16 GB / 50 GB, IPv4, always on | 8 × 820 + 16 × 223 + 837 = 6,560 + 3,568 + 837 | **10,965 ₽** |
| (d) standard, 62.5% of hours | 10,128 × 0.625 + 837 = 6,330 + 837 | **7,167 ₽** |
| (d) high-frequency, 8 vCPU / 16 GB / 50 GB, IPv4, always on | 8 × 1,297 + 16 × 223 + 837 = 10,376 + 3,568 + 837 | **14,781 ₽** |
| (d) high-frequency, 62.5% of hours | 13,944 × 0.625 + 837 = 8,715 + 837 | **9,552 ₽** |
| (e) Dedicated 8-core server, Selectel AR31-NVMe (Ryzen 7 3700X, 32 GB, 2 × 500 GB NVMe), unlimited 1 Gbit/s | list price | **≈11,000 ₽** (**third-party**, **unconfirmed**); official alternatives at Timeweb: 8-core E-2288G / 64 GB 18,580 ₽, 10-core i9-10900K / 64 GB 15,900 ₽ |
| (f) Managed PostgreSQL, Yandex Cloud `s3-c2-m8` (2 vCPU / 8 GB), 20 GB `network-ssd`, one host | 2 × 1,353.02 + 8 × 365.18 + 20 × 15.70 = 2,706.04 + 2,921.44 + 313.92 | **≈5,941 ₽** (derived); backups within the free allowance |
| (g) Object storage 10 GB, Yandex standard | (10 − 1) × 2.376 | **≈21 ₽** |
| (g) Object storage 200 GB | 199 × 2.376 | **≈473 ₽** (cold class 200 × 1.267 ≈ 253 ₽; VK Hotbox 200 × 2.28 = 456 ₽) |
| (g) Object storage 2 TB (2,048 GB) | 2,047 × 2.376 | **≈4,864 ₽** (cold ≈2,595 ₽; VK Hotbox ≈4,669 ₽); egress extra at 1.68 ₽ per GB after 100 GB at Yandex |
| (h) Email, 2,000 a month | Postbox: within the free 2,000; Unisender Go: 6,000 plan | Postbox **0 ₽**; Unisender Go **800 ₽** (0 ₽ the first 2 months) |
| (h) Email, 10,000 a month | Postbox: 8 × 80.32; Unisender Go: 10,000 plan | Postbox **≈643 ₽**; Unisender Go **1,200 ₽** |
| (h) Email, 50,000 a month | Postbox: 642.56 + 40 × 70.15; Unisender Go: 50,000 plan | Postbox **≈3,449 ₽**; Unisender Go **2,750 ₽** |

Postbox needs its default quota of 200 emails a day raised above about 6,000 a month; Unisender Go's 6,000 plan caps sending at 500 a day until raised.

## What changed since October

- No unit price of VK Cloud, Yandex Cloud, Unisender Go or Selectel's "from" prices moved; every October building block that used them holds.
- New facts: Yandex Cloud's October outage, free October and backups enabled by default for new VMs; Yandex charges 434.81 ₽ a month for a static IP on a stopped VM; Selectel bills stopped cloud servers in full unless frozen; Timeweb Cloud's official Premium prices (2 / 4 at 3,840 ₽, 4 / 8 at 7,000 ₽) are far above the third-party figures October used; official Timeweb dedicated prices for 8 to 10 cores (15,900–18,580 ₽); REG.RU renewal about 1,199 ₽ and RU-CENTER registration 747 ₽; Let's Encrypt added sanctions screening in June 2026 but still issues for ordinary .ru domains.

## Not verified

- Yandex Cloud ruble prices as stated figures (derived from USD rows), the price and opt-out of default VM backups, and every figure on the share of data recovered or lost in the October incident, which come only from third-party news.
- Selectel cloud unit prices, dedicated server list prices (the official tables showed "prices are being updated"), the configuration behind "from 948.50 ₽".
- Whether VK Cloud's Cloud Databases rows apply to its managed PostgreSQL, and whether a 1 vCPU / 1 GB flavour is orderable.
- Timeweb Cloud VAT treatment and whether its Premium line was repriced since September 2026.
- REG.RU and RU-CENTER .ru renewal prices on an official page; VAT on domain prices.
- The sanctions text of Let's Encrypt subscriber agreement version 1.8.
- Any provider price change announced for 2027.
