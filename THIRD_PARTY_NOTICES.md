# Third-Party Notices

Amber includes or links to third-party software. The dependency lockfiles are
the authoritative version inventory for each release. This summary covers the
principal runtime components and does not replace their license texts.

| Component | Role | License |
|---|---|---|
| Tauri and Tauri plugins | Desktop runtime and native integration | Apache-2.0 OR MIT |
| Vue, Vue Router, Pinia, Vue I18n | User interface | MIT |
| modernc.org/sqlite and modernc runtime packages | SQLite storage | BSD-3-Clause and component-specific notices |
| refraction-networking/utls | TLS compatibility transport | BSD-3-Clause |
| golang.org/x/crypto, x/net, x/sys | Go networking and platform support | BSD-3-Clause |
| pelletier/go-toml | TOML parsing | MIT |
| golang-jwt/jwt | JWT processing | MIT |
| google/uuid | UUID generation | BSD-3-Clause |

The gateway incorporates ideas and adapted behavior from the `sub2api`
project, identified by the upstream project as LGPL v3. Modified LGPL-covered
source must remain available under the LGPL when binaries are distributed.

Before publishing a release, review `package-lock.json`, `core/go.sum`, and
`src-tauri/Cargo.lock` for newly introduced packages and update this notice.
