# BigBlueButton probe prerequisites checked on 2026-11-05

Source: [official installation guide](https://docs.bigbluebutton.org/administration/install/). This is documentation research, not a live server test or a purchase recommendation.

The guide currently describes BigBlueButton 4.0 on dedicated clean Ubuntu 24.04, with Docker. Its production recommendation is 8 CPU cores and 16 GB RAM; disk is 50 GB without recordings. It lists TCP 80/443, TCP and UDP 3478 for built-in TURN, UDP 16384–32768, a TLS hostname and symmetric 250 Mbit/s bandwidth. The reduced development profile is 4 cores/8 GB/50 GB. Verify the exact release and installation artifacts again at provisioning; no installation script was executed.

[ADR-0013](../../docs/decisions/0013-pilot-deployment.md) already accepts the smaller dedicated Russian-region pilot host for two simultaneous one-to-one lessons, contingent on the live probe. This is not production-capacity evidence. The additional TURN port must be included in the later exact firewall plan; do not reuse an older abbreviated port list blindly.

No remote test server is currently available, as confirmed by the owner. Required inputs before provisioning are a Russian-region host, approved cost and ownership, domain/DNS access, SSH public-key access restricted to operator addresses, and an approved secret-management path. No secret should be sent in chat or committed. The operator supplies credentials outside the repository; remote access and any secret loading require a separately reviewed mechanism, not an extension of the local dev-env exception by assumption.
