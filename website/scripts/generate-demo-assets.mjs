import { chromium } from '@playwright/test';
import { mkdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const websiteDirectory = path.resolve(scriptDirectory, '..');
const outputDirectory = path.join(websiteDirectory, 'public', 'screenshots', 'v044');
const markPath = path.join(websiteDirectory, 'public', 'amber-mark.svg');
const ogPath = path.join(websiteDirectory, 'public', 'og-cover-v044.png');

const markSvg = await readFile(markPath, 'utf8');
const markUri = 'data:image/svg+xml;base64,' + Buffer.from(markSvg).toString('base64');

const styles = String.raw`
  :root {
    color-scheme: light;
    --canvas: #e7e7e3;
    --app: #f7f7f4;
    --sidebar: #eeece5;
    --surface: #ffffff;
    --surface-soft: #f1f0eb;
    --ink: #202522;
    --muted: #767a73;
    --faint: #a5a79f;
    --border: #dcdcd5;
    --amber: #d8643f;
    --amber-dark: #a94327;
    --amber-soft: #f7e8e1;
    --teal: #167f78;
    --teal-soft: #e3f2ef;
    --green: #287a52;
    --green-soft: #e5f3eb;
    --yellow: #ad7422;
    --yellow-soft: #f8eedc;
    --red: #b64b40;
    --red-soft: #fae9e7;
    font-family: "Segoe UI", "Microsoft YaHei UI", "Microsoft YaHei", Arial, sans-serif;
  }

  * {
    box-sizing: border-box;
  }

  html,
  body {
    width: 100%;
    height: 100%;
    margin: 0;
  }

  body {
    overflow: hidden;
    background: var(--canvas);
    color: var(--ink);
    font-size: 14px;
    letter-spacing: 0;
  }

  button,
  input {
    font: inherit;
  }

  .canvas {
    width: 100vw;
    height: 100vh;
    padding: 20px;
  }

  .app-window {
    display: grid;
    width: 100%;
    height: 100%;
    grid-template-columns: 208px minmax(0, 1fr);
    overflow: hidden;
    border: 1px solid #d0d1cb;
    border-radius: 10px;
    background: var(--app);
    box-shadow: 0 22px 50px rgba(30, 34, 31, 0.13);
  }

  .sidebar {
    display: flex;
    min-height: 0;
    flex-direction: column;
    border-right: 1px solid var(--border);
    background: var(--sidebar);
    padding: 21px 13px 16px;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 0 9px 23px;
  }

  .brand img,
  .mobile-bar img {
    display: block;
    width: 38px;
    height: 38px;
  }

  .brand-name {
    font-size: 17px;
    font-weight: 760;
    line-height: 1.2;
  }

  .brand-subtitle {
    margin-top: 2px;
    color: var(--muted);
    font-size: 11px;
  }

  .nav {
    display: grid;
    gap: 4px;
  }

  .nav-item {
    display: grid;
    min-height: 42px;
    grid-template-columns: 28px 1fr;
    align-items: center;
    gap: 6px;
    border-radius: 7px;
    color: #62665f;
    padding: 0 10px;
    font-size: 13px;
    font-weight: 620;
  }

  .nav-item.active {
    position: relative;
    background: #f7e4db;
    color: var(--amber-dark);
  }

  .nav-item.active::before {
    position: absolute;
    left: -13px;
    width: 4px;
    height: 25px;
    border-radius: 0 4px 4px 0;
    background: var(--amber);
    content: "";
  }

  .nav-symbol {
    display: grid;
    width: 25px;
    height: 25px;
    place-items: center;
    border: 1px solid transparent;
    border-radius: 6px;
    font-size: 15px;
    font-weight: 760;
  }

  .nav-item.active .nav-symbol {
    border-color: #efc4b3;
    background: #fff9f6;
  }

  .sidebar-bottom {
    display: grid;
    gap: 11px;
    margin-top: auto;
    padding: 16px 10px 2px;
    color: var(--muted);
    font-size: 11px;
  }

  .sidebar-status {
    display: flex;
    align-items: center;
    gap: 7px;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    flex: none;
    border-radius: 50%;
    background: #36a469;
    box-shadow: 0 0 0 4px rgba(54, 164, 105, 0.12);
  }

  .version-line {
    padding-top: 10px;
    border-top: 1px solid var(--border);
  }

  .mobile-bar {
    display: none;
  }

  .main {
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    padding: 24px 28px 26px;
  }

  .page-head {
    display: flex;
    min-height: 57px;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 16px;
  }

  .page-title {
    margin: 0;
    color: #171b19;
    font-size: 25px;
    font-weight: 760;
    line-height: 1.2;
  }

  .page-subtitle {
    margin: 6px 0 0;
    color: var(--muted);
    font-size: 13px;
    line-height: 1.5;
  }

  .head-actions,
  .button-row,
  .chip-row,
  .row-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .demo-badge {
    display: inline-flex;
    min-height: 30px;
    align-items: center;
    border: 1px solid #dbc9bd;
    border-radius: 999px;
    background: #fffaf7;
    color: #83513b;
    padding: 0 12px;
    font-size: 11px;
    font-weight: 720;
    white-space: nowrap;
  }

  .button {
    display: inline-flex;
    min-height: 36px;
    align-items: center;
    justify-content: center;
    gap: 7px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    padding: 0 14px;
    font-size: 12px;
    font-weight: 700;
    white-space: nowrap;
  }

  .button.primary {
    border-color: var(--amber);
    background: var(--amber);
    color: #fff;
  }

  .button.teal {
    border-color: var(--teal);
    background: var(--teal);
    color: #fff;
  }

  .button.icon {
    width: 34px;
    padding: 0;
  }

  .service-strip {
    display: flex;
    min-height: 67px;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    margin-bottom: 16px;
    border: 1px solid #b9dcd4;
    border-radius: 8px;
    background: var(--teal-soft);
    padding: 12px 16px;
  }

  .service-title {
    display: flex;
    align-items: center;
    gap: 9px;
    color: #125d58;
    font-size: 14px;
    font-weight: 760;
  }

  .service-copy {
    margin: 4px 0 0 17px;
    color: #4e6d69;
    font-size: 11px;
  }

  .chip {
    display: inline-flex;
    min-height: 27px;
    align-items: center;
    gap: 6px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    color: #5a5f59;
    padding: 0 10px;
    font-size: 11px;
    font-weight: 650;
    white-space: nowrap;
  }

  .chip.teal {
    border-color: #b6dcd4;
    background: #f5fbfa;
    color: #176b65;
  }

  .chip.amber {
    border-color: #e9c4b4;
    background: #fff8f5;
    color: #9b4b2f;
  }

  .metric-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  .metric {
    min-width: 0;
    min-height: 91px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    padding: 14px 16px;
  }

  .metric-label {
    color: var(--muted);
    font-size: 11px;
  }

  .metric-value {
    margin-top: 8px;
    font-size: 25px;
    font-weight: 760;
    line-height: 1;
  }

  .metric-note {
    margin-top: 7px;
    color: var(--green);
    font-size: 10px;
  }

  .two-column {
    display: grid;
    grid-template-columns: minmax(0, 1.12fr) minmax(360px, 0.88fr);
    gap: 14px;
  }

  .panel {
    min-width: 0;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }

  .panel-head {
    display: flex;
    min-height: 52px;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    border-bottom: 1px solid #e6e6e0;
    padding: 0 16px;
  }

  .panel-title {
    font-size: 13px;
    font-weight: 760;
  }

  .panel-note {
    color: var(--muted);
    font-size: 10px;
  }

  .text-link {
    color: var(--amber-dark);
    font-size: 11px;
    font-weight: 700;
  }

  .account-compact,
  .activity-row {
    display: grid;
    align-items: center;
    border-bottom: 1px solid #ededE8;
  }

  .account-compact:last-child,
  .activity-row:last-child {
    border-bottom: 0;
  }

  .account-compact {
    min-height: 73px;
    grid-template-columns: minmax(150px, 1.15fr) 88px minmax(180px, 0.85fr);
    gap: 14px;
    padding: 10px 16px;
  }

  .account-name {
    font-size: 12px;
    font-weight: 720;
  }

  .account-meta {
    margin-top: 4px;
    color: var(--muted);
    font-size: 10px;
  }

  .state {
    display: inline-flex;
    width: fit-content;
    min-height: 24px;
    align-items: center;
    gap: 6px;
    border-radius: 999px;
    background: var(--green-soft);
    color: var(--green);
    padding: 0 9px;
    font-size: 10px;
    font-weight: 720;
  }

  .state::before {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
    content: "";
  }

  .state.amber-state {
    background: var(--yellow-soft);
    color: var(--yellow);
  }

  .usage {
    min-width: 0;
  }

  .usage-top {
    display: flex;
    justify-content: space-between;
    gap: 10px;
    color: var(--muted);
    font-size: 10px;
  }

  .usage-top strong {
    color: var(--ink);
    font-weight: 720;
  }

  .bar {
    height: 6px;
    margin-top: 7px;
    overflow: hidden;
    border-radius: 999px;
    background: #e9e8e2;
  }

  .bar > span {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--amber);
  }

  .bar.teal > span {
    background: var(--teal);
  }

  .activity-row {
    min-height: 58px;
    grid-template-columns: 52px minmax(0, 1fr) 88px 66px;
    gap: 10px;
    padding: 8px 16px;
    font-size: 11px;
  }

  .code-ok {
    display: inline-grid;
    min-height: 24px;
    place-items: center;
    border-radius: 6px;
    background: var(--green-soft);
    color: var(--green);
    font-size: 10px;
    font-weight: 760;
  }

  .activity-name {
    font-weight: 700;
  }

  .activity-sub,
  .activity-value {
    color: var(--muted);
    font-size: 10px;
  }

  .toolbar {
    display: flex;
    min-height: 55px;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    margin-bottom: 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    padding: 9px 12px;
  }

  .segmented {
    display: inline-flex;
    min-height: 36px;
    align-items: center;
    border: 1px solid var(--border);
    border-radius: 7px;
    background: var(--surface-soft);
    padding: 3px;
  }

  .segment {
    display: inline-flex;
    min-width: 96px;
    min-height: 28px;
    align-items: center;
    justify-content: center;
    border-radius: 5px;
    color: var(--muted);
    font-size: 11px;
    font-weight: 690;
  }

  .segment.active {
    border: 1px solid #deded8;
    background: #fff;
    color: var(--ink);
    box-shadow: 0 2px 5px rgba(30, 35, 31, 0.07);
  }

  .list-panel {
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }

  .account-row {
    display: grid;
    min-height: 137px;
    grid-template-columns: 46px minmax(190px, 1.1fr) 100px minmax(230px, 1.25fr) 122px;
    align-items: center;
    gap: 17px;
    border-bottom: 1px solid #e8e8e2;
    padding: 15px 18px;
  }

  .account-row:last-child {
    border-bottom: 0;
  }

  .account-avatar {
    display: grid;
    width: 42px;
    height: 42px;
    place-items: center;
    border-radius: 8px;
    background: var(--amber-soft);
    color: var(--amber-dark);
    font-size: 19px;
    font-weight: 760;
  }

  .account-avatar.teal-avatar {
    background: var(--teal-soft);
    color: var(--teal);
  }

  .account-avatar.neutral-avatar {
    background: #eeeeea;
    color: #5b605b;
  }

  .small-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 7px;
  }

  .tiny-tag {
    display: inline-flex;
    min-height: 22px;
    align-items: center;
    border-radius: 999px;
    background: var(--surface-soft);
    color: #696d67;
    padding: 0 8px;
    font-size: 9px;
    font-weight: 650;
  }

  .quota-stack {
    display: grid;
    gap: 12px;
  }

  .quota-line .usage-top {
    font-size: 9px;
  }

  .row-actions {
    justify-content: flex-end;
  }

  .toggle {
    position: relative;
    width: 38px;
    height: 22px;
    flex: none;
    border-radius: 999px;
    background: var(--amber);
  }

  .toggle::after {
    position: absolute;
    top: 3px;
    right: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: #fff;
    content: "";
  }

  .overview-band {
    display: grid;
    grid-template-columns: 1.1fr 0.9fr 0.9fr;
    gap: 12px;
    margin-bottom: 13px;
  }

  .overview-cell {
    min-height: 76px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    padding: 13px 15px;
  }

  .overview-cell strong {
    display: block;
    margin-top: 7px;
    font-size: 16px;
  }

  .route-row {
    display: grid;
    min-height: 119px;
    grid-template-columns: minmax(220px, 1.1fr) 120px minmax(190px, 0.9fr) 150px;
    align-items: center;
    gap: 22px;
    border-bottom: 1px solid #e8e8e2;
    padding: 15px 20px;
  }

  .route-row:last-child {
    border-bottom: 0;
  }

  .route-icon {
    display: inline-grid;
    width: 34px;
    height: 34px;
    margin-right: 10px;
    place-items: center;
    border-radius: 7px;
    background: var(--teal-soft);
    color: var(--teal);
    font-weight: 800;
    vertical-align: middle;
  }

  .route-name {
    vertical-align: middle;
    font-weight: 730;
  }

  .route-sub {
    margin: 6px 0 0 46px;
    color: var(--muted);
    font-size: 10px;
  }

  .health {
    display: flex;
    align-items: center;
    gap: 9px;
  }

  .health-ring {
    display: grid;
    width: 39px;
    height: 39px;
    place-items: center;
    border: 5px solid #c8e5de;
    border-top-color: var(--teal);
    border-radius: 50%;
    color: #346d68;
    font-size: 8px;
    font-weight: 760;
  }

  .cloud-grid {
    display: grid;
    grid-template-columns: minmax(0, 0.85fr) minmax(0, 1.15fr);
    gap: 14px;
  }

  .sync-hero {
    min-height: 113px;
    margin-bottom: 14px;
    border: 1px solid #b9dcd4;
    border-radius: 8px;
    background: var(--teal-soft);
    padding: 18px;
  }

  .sync-title {
    display: flex;
    align-items: center;
    gap: 10px;
    color: #155f59;
    font-size: 15px;
    font-weight: 760;
  }

  .shield {
    display: grid;
    width: 35px;
    height: 35px;
    place-items: center;
    border-radius: 8px;
    background: var(--teal);
    color: #fff;
    font-size: 17px;
  }

  .sync-copy {
    margin: 9px 0 0 45px;
    color: #4e6e69;
    font-size: 11px;
    line-height: 1.55;
  }

  .definition-list {
    display: grid;
  }

  .definition-row {
    display: flex;
    min-height: 57px;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    border-bottom: 1px solid #e8e8e2;
    padding: 10px 16px;
  }

  .definition-row:last-child {
    border-bottom: 0;
  }

  .definition-label {
    color: var(--muted);
    font-size: 10px;
  }

  .definition-value {
    margin-top: 4px;
    font-size: 12px;
    font-weight: 700;
  }

  .sharing-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 18px;
  }

  .sharing-title {
    font-size: 15px;
    font-weight: 760;
  }

  .sharing-copy {
    margin-top: 5px;
    color: var(--muted);
    font-size: 11px;
  }

  .share-callout {
    margin: 0 18px 14px;
    border: 1px solid #e8c5b5;
    border-radius: 8px;
    background: var(--amber-soft);
    padding: 14px 15px;
  }

  .share-callout strong {
    color: #8e432c;
    font-size: 12px;
  }

  .share-callout p {
    margin: 5px 0 0;
    color: #885c4d;
    font-size: 10px;
    line-height: 1.5;
  }

  .limits {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
    padding: 0 18px 14px;
  }

  .limit {
    min-height: 74px;
    border: 1px solid var(--border);
    border-radius: 7px;
    background: var(--surface-soft);
    padding: 11px;
  }

  .limit strong {
    display: block;
    margin-top: 7px;
    font-size: 15px;
  }

  .share-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    border-top: 1px solid #e8e8e2;
    padding: 14px 18px;
  }

  .codex-tabs {
    margin-bottom: 13px;
  }

  .connection-panel {
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    padding: 18px;
  }

  .connection-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    margin-bottom: 17px;
  }

  .connection-title {
    font-size: 15px;
    font-weight: 760;
  }

  .connection-copy {
    margin-top: 5px;
    color: var(--muted);
    font-size: 11px;
  }

  .flow {
    display: grid;
    grid-template-columns: 1fr 54px 1fr 54px 1fr;
    align-items: center;
    margin-bottom: 17px;
  }

  .flow-node {
    min-height: 102px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface-soft);
    padding: 15px;
  }

  .flow-node.active {
    border-color: #e4b9a8;
    background: var(--amber-soft);
  }

  .flow-node.teal-node {
    border-color: #b7dcd5;
    background: var(--teal-soft);
  }

  .flow-icon {
    display: grid;
    width: 30px;
    height: 30px;
    place-items: center;
    margin-bottom: 10px;
    border-radius: 7px;
    background: #fff;
    color: var(--amber-dark);
    font-weight: 800;
  }

  .flow-node.teal-node .flow-icon {
    color: var(--teal);
  }

  .flow-title {
    font-size: 12px;
    font-weight: 740;
  }

  .flow-copy {
    margin-top: 4px;
    color: var(--muted);
    font-size: 9px;
  }

  .flow-arrow {
    color: var(--faint);
    text-align: center;
    font-size: 22px;
  }

  .config-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 10px;
  }

  .config-cell {
    min-height: 70px;
    border-top: 1px solid var(--border);
    padding: 14px 3px 0;
  }

  .config-cell strong {
    display: block;
    margin-top: 6px;
    font-size: 12px;
  }

  .injection-status {
    display: flex;
    min-height: 65px;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-top: 13px;
    border: 1px solid #b9dcd4;
    border-radius: 8px;
    background: var(--teal-soft);
    padding: 12px 16px;
  }

  .injection-status strong {
    color: #155f59;
  }

  .injection-status p {
    margin: 4px 0 0;
    color: #4e6e69;
    font-size: 10px;
  }

  .mobile .canvas {
    padding: 10px;
  }

  .mobile .app-window {
    grid-template-columns: 1fr;
    grid-template-rows: 52px minmax(0, 1fr);
    border-radius: 8px;
  }

  .mobile .sidebar {
    display: none;
  }

  .mobile .mobile-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
    background: var(--sidebar);
    padding: 7px 12px;
  }

  .mobile .mobile-brand {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    font-weight: 760;
  }

  .mobile .mobile-bar img {
    width: 30px;
    height: 30px;
  }

  .mobile .main {
    padding: 14px;
  }

  .mobile .page-head {
    min-height: 46px;
    align-items: flex-start;
    margin-bottom: 10px;
  }

  .mobile .page-title {
    font-size: 20px;
  }

  .mobile .page-subtitle {
    max-width: 300px;
    margin-top: 3px;
    font-size: 10px;
  }

  .mobile .head-actions .button,
  .mobile .head-actions .demo-badge {
    display: none;
  }

  .mobile .service-strip {
    min-height: 56px;
    margin-bottom: 10px;
    padding: 9px 12px;
  }

  .mobile .service-title {
    font-size: 12px;
  }

  .mobile .service-copy {
    margin-top: 2px;
    font-size: 9px;
  }

  .mobile .service-strip .chip-row {
    display: none;
  }

  .mobile .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    margin-bottom: 10px;
  }

  .mobile .metric {
    min-height: 72px;
    padding: 10px 12px;
  }

  .mobile .metric-value {
    margin-top: 6px;
    font-size: 20px;
  }

  .mobile .metric-note {
    margin-top: 5px;
    font-size: 8px;
  }

  .mobile .two-column {
    grid-template-columns: 1fr;
    gap: 0;
  }

  .mobile .activity-panel {
    display: none;
  }

  .mobile .panel-head {
    min-height: 43px;
    padding: 0 12px;
  }

  .mobile .account-compact {
    min-height: 58px;
    grid-template-columns: minmax(120px, 1fr) 72px minmax(120px, 0.8fr);
    gap: 8px;
    padding: 8px 12px;
  }

  .mobile .account-meta,
  .mobile .usage-top {
    font-size: 8px;
  }
`;

const pageTemplate = String.raw`
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>__STYLES__</style>
  </head>
  <body class="__BODY_CLASS__">__BODY__</body>
</html>
`;

const shellTemplate = String.raw`
<div class="canvas">
  <div class="app-window">
    <aside class="sidebar">
      <div class="brand">
        <img src="__MARK__" alt="">
        <div>
          <div class="brand-name">Amber</div>
          <div class="brand-subtitle">Windows Codex 网关</div>
        </div>
      </div>
      <nav class="nav">__NAV__</nav>
      <div class="sidebar-bottom">
        <div class="sidebar-status"><span class="status-dot"></span>服务运行中</div>
        <div class="version-line">演示数据 · v0.4.4</div>
      </div>
    </aside>
    <div class="mobile-bar">
      <div class="mobile-brand"><img src="__MARK__" alt="">Amber</div>
      <span class="demo-badge">演示数据 · v0.4.4</span>
    </div>
    <main class="main">
      <header class="page-head">
        <div>
          <h1 class="page-title">__TITLE__</h1>
          <p class="page-subtitle">__SUBTITLE__</p>
        </div>
        <div class="head-actions">
          <span class="demo-badge">演示数据 · v0.4.4</span>
          __ACTIONS__
        </div>
      </header>
      __CONTENT__
    </main>
  </div>
</div>
`;

const navItems = [
  ['dashboard', '▦', '仪表盘'],
  ['accounts', '◎', '账号调度'],
  ['network', '↗', '代理与网络'],
  ['sharing', '◇', '云账户与共享'],
  ['codex', '>_', 'Codex 接入'],
  ['docs', '≡', '使用文档'],
];

function navMarkup(active) {
  return navItems
    .map(([key, symbol, label]) => {
      const activeClass = key === active ? ' active' : '';
      return '<div class="nav-item' + activeClass + '"><span class="nav-symbol">' + symbol + '</span><span>' + label + '</span></div>';
    })
    .join('');
}

function shell(scene) {
  return shellTemplate
    .replaceAll('__MARK__', markUri)
    .replace('__NAV__', navMarkup(scene.active))
    .replace('__TITLE__', scene.title)
    .replace('__SUBTITLE__', scene.subtitle)
    .replace('__ACTIONS__', scene.actions)
    .replace('__CONTENT__', scene.content);
}

function documentFor(body, bodyClass = 'desktop') {
  return pageTemplate
    .replace('__STYLES__', styles)
    .replace('__BODY_CLASS__', bodyClass)
    .replace('__BODY__', body);
}

const dashboardContent = String.raw`
<section class="service-strip">
  <div>
    <div class="service-title"><span class="status-dot"></span>本地网关运行中</div>
    <div class="service-copy">账号池已就绪，Codex 请求按可用额度自动调度</div>
  </div>
  <div class="chip-row">
    <span class="chip teal">本地优先</span>
    <span class="chip">自动切换</span>
    <span class="chip amber">共享连接 1 路</span>
  </div>
</section>
<section class="metric-grid">
  <article class="metric">
    <div class="metric-label">可用账号</div>
    <div class="metric-value">3</div>
    <div class="metric-note">全部状态正常</div>
  </article>
  <article class="metric">
    <div class="metric-label">今日请求</div>
    <div class="metric-value">248</div>
    <div class="metric-note">调度稳定</div>
  </article>
  <article class="metric">
    <div class="metric-label">额度使用</div>
    <div class="metric-value">37%</div>
    <div class="metric-note">余量充足</div>
  </article>
  <article class="metric">
    <div class="metric-label">平均响应</div>
    <div class="metric-value">684 ms</div>
    <div class="metric-note">连接正常</div>
  </article>
</section>
<section class="two-column dashboard-columns">
  <div class="panel">
    <div class="panel-head">
      <div>
        <div class="panel-title">账号池</div>
        <div class="panel-note">额度感知与自动切换</div>
      </div>
      <span class="text-link">管理账号</span>
    </div>
    <div class="account-compact">
      <div><div class="account-name">主力额度</div><div class="account-meta">优先调度 · 并发 1 / 3</div></div>
      <span class="state">正常</span>
      <div class="usage"><div class="usage-top"><span>窗口用量</span><strong>28%</strong></div><div class="bar"><span style="width:28%"></span></div></div>
    </div>
    <div class="account-compact">
      <div><div class="account-name">备用额度</div><div class="account-meta">自动接管 · 并发 0 / 2</div></div>
      <span class="state">正常</span>
      <div class="usage"><div class="usage-top"><span>窗口用量</span><strong>16%</strong></div><div class="bar teal"><span style="width:16%"></span></div></div>
    </div>
    <div class="account-compact">
      <div><div class="account-name">共享额度</div><div class="account-meta">受控访问 · 并发 0 / 1</div></div>
      <span class="state amber-state">待命</span>
      <div class="usage"><div class="usage-top"><span>共享限额</span><strong>42%</strong></div><div class="bar"><span style="width:42%"></span></div></div>
    </div>
  </div>
  <div class="panel activity-panel">
    <div class="panel-head">
      <div>
        <div class="panel-title">最近请求</div>
        <div class="panel-note">仅展示脱敏运行状态</div>
      </div>
      <span class="text-link">查看统计</span>
    </div>
    <div class="activity-row"><span class="code-ok">200</span><div><div class="activity-name">Codex 对话</div><div class="activity-sub">主力额度</div></div><span class="activity-value">5.2K tok</span><span class="activity-value">612 ms</span></div>
    <div class="activity-row"><span class="code-ok">200</span><div><div class="activity-name">远程注入</div><div class="activity-sub">备用额度</div></div><span class="activity-value">3.8K tok</span><span class="activity-value">731 ms</span></div>
    <div class="activity-row"><span class="code-ok">200</span><div><div class="activity-name">共享连接</div><div class="activity-sub">受控访问</div></div><span class="activity-value">1.6K tok</span><span class="activity-value">698 ms</span></div>
    <div class="activity-row"><span class="code-ok">200</span><div><div class="activity-name">Codex 对话</div><div class="activity-sub">主力额度</div></div><span class="activity-value">4.1K tok</span><span class="activity-value">654 ms</span></div>
  </div>
</section>
`;

const accountsContent = String.raw`
<section class="toolbar">
  <div class="segmented">
    <span class="segment active">全部 3</span>
    <span class="segment">可用 3</span>
    <span class="segment">调度中 2</span>
  </div>
  <div class="button-row">
    <span class="button">↻ 批量测试</span>
    <span class="button primary">↑ 导入账号</span>
  </div>
</section>
<section class="list-panel">
  <article class="account-row">
    <div class="account-avatar">●</div>
    <div>
      <div class="account-name">主力额度</div>
      <div class="small-tags"><span class="tiny-tag">Plus</span><span class="tiny-tag">优先级 1</span><span class="tiny-tag">自动调度</span></div>
      <div class="account-meta">最近测试成功 · 并发 1 / 3</div>
    </div>
    <span class="state">正常</span>
    <div class="quota-stack">
      <div class="quota-line"><div class="usage-top"><span>短窗口用量</span><strong>28%</strong></div><div class="bar"><span style="width:28%"></span></div></div>
      <div class="quota-line"><div class="usage-top"><span>周窗口用量</span><strong>19%</strong></div><div class="bar teal"><span style="width:19%"></span></div></div>
    </div>
    <div class="row-actions"><span class="toggle"></span><span class="button icon">↻</span><span class="button icon">•••</span></div>
  </article>
  <article class="account-row">
    <div class="account-avatar teal-avatar">●</div>
    <div>
      <div class="account-name">备用额度</div>
      <div class="small-tags"><span class="tiny-tag">Team</span><span class="tiny-tag">优先级 2</span><span class="tiny-tag">故障接管</span></div>
      <div class="account-meta">最近测试成功 · 并发 0 / 2</div>
    </div>
    <span class="state">正常</span>
    <div class="quota-stack">
      <div class="quota-line"><div class="usage-top"><span>短窗口用量</span><strong>16%</strong></div><div class="bar"><span style="width:16%"></span></div></div>
      <div class="quota-line"><div class="usage-top"><span>周窗口用量</span><strong>11%</strong></div><div class="bar teal"><span style="width:11%"></span></div></div>
    </div>
    <div class="row-actions"><span class="toggle"></span><span class="button icon">↻</span><span class="button icon">•••</span></div>
  </article>
  <article class="account-row">
    <div class="account-avatar neutral-avatar">●</div>
    <div>
      <div class="account-name">共享额度</div>
      <div class="small-tags"><span class="tiny-tag">Plus</span><span class="tiny-tag">优先级 3</span><span class="tiny-tag">受控共享</span></div>
      <div class="account-meta">最近测试成功 · 并发 0 / 1</div>
    </div>
    <span class="state amber-state">待命</span>
    <div class="quota-stack">
      <div class="quota-line"><div class="usage-top"><span>短窗口用量</span><strong>42%</strong></div><div class="bar"><span style="width:42%"></span></div></div>
      <div class="quota-line"><div class="usage-top"><span>共享限额</span><strong>35%</strong></div><div class="bar teal"><span style="width:35%"></span></div></div>
    </div>
    <div class="row-actions"><span class="toggle"></span><span class="button icon">↻</span><span class="button icon">•••</span></div>
  </article>
</section>
`;

const networkContent = String.raw`
<section class="toolbar">
  <div>
    <div class="panel-title">默认网络模式</div>
    <div class="panel-note">按账号覆盖，未绑定时使用默认策略</div>
  </div>
  <div class="segmented">
    <span class="segment">Direct</span>
    <span class="segment">System</span>
    <span class="segment active">Proxy</span>
  </div>
  <span class="button primary">＋ 添加出口</span>
</section>
<section class="overview-band">
  <article class="overview-cell"><span class="metric-label">已绑定账号</span><strong>3 / 3</strong><div class="metric-note">策略已应用</div></article>
  <article class="overview-cell"><span class="metric-label">连接测试</span><strong>全部正常</strong><div class="metric-note">最近检查完成</div></article>
  <article class="overview-cell"><span class="metric-label">自动回退</span><strong>已启用</strong><div class="metric-note">网络切换受控</div></article>
</section>
<section class="list-panel">
  <article class="route-row">
    <div><span class="route-icon">↗</span><span class="route-name">工作出口</span><div class="route-sub">SOCKS5 · 连接信息已保护</div></div>
    <span class="state">正常</span>
    <div class="usage"><div class="usage-top"><span>绑定账号</span><strong>2</strong></div><div class="bar teal"><span style="width:66%"></span></div></div>
    <div class="row-actions"><span class="button">↻ 测试</span><span class="button icon">•••</span></div>
  </article>
  <article class="route-row">
    <div><span class="route-icon">↗</span><span class="route-name">备用出口</span><div class="route-sub">HTTPS · 连接信息已保护</div></div>
    <span class="state">正常</span>
    <div class="usage"><div class="usage-top"><span>绑定账号</span><strong>1</strong></div><div class="bar"><span style="width:33%"></span></div></div>
    <div class="row-actions"><span class="button">↻ 测试</span><span class="button icon">•••</span></div>
  </article>
  <article class="route-row">
    <div><span class="route-icon">⌁</span><span class="route-name">系统网络</span><div class="route-sub">跟随 Windows 系统设置</div></div>
    <span class="state amber-state">待命</span>
    <div class="health"><span class="health-ring">备用</span><div><div class="account-name">自动回退</div><div class="account-meta">出口不可用时接管</div></div></div>
    <div class="row-actions"><span class="button">设为默认</span><span class="button icon">•••</span></div>
  </article>
</section>
`;

const sharingContent = String.raw`
<section class="sync-hero">
  <div class="sync-title"><span class="shield">✓</span>加密云同步已启用</div>
  <div class="sync-copy">账号配置在本地加密后同步，云端仅保存密文；共享访问与个人配置相互隔离。</div>
</section>
<section class="cloud-grid">
  <div class="panel">
    <div class="panel-head">
      <div><div class="panel-title">独立云工作区</div><div class="panel-note">配置同步状态</div></div>
      <span class="state">已同步</span>
    </div>
    <div class="definition-list">
      <div class="definition-row"><div><div class="definition-label">账号配置</div><div class="definition-value">3 项已同步</div></div><span class="chip teal">密文存储</span></div>
      <div class="definition-row"><div><div class="definition-label">代理策略</div><div class="definition-value">2 项已同步</div></div><span class="chip teal">密文存储</span></div>
      <div class="definition-row"><div><div class="definition-label">同步状态</div><div class="definition-value">当前内容已是最新</div></div><span class="button">↻ 立即同步</span></div>
      <div class="definition-row"><div><div class="definition-label">恢复保护</div><div class="definition-value">主密码不上传</div></div><span class="chip">本地解密</span></div>
    </div>
  </div>
  <div class="panel">
    <div class="sharing-head">
      <div><div class="sharing-title">好友账号受控共享</div><div class="sharing-copy">独立访问凭据、限流、额度和随时撤销</div></div>
      <span class="state">共享中</span>
    </div>
    <div class="share-callout">
      <strong>共享连接已创建</strong>
      <p>连接信息已隐藏。好友仅能使用分配额度，无法查看账号登录信息或个人云配置。</p>
    </div>
    <div class="limits">
      <div class="limit"><span class="metric-label">共享账号</span><strong>2 个</strong><div class="metric-note">账号池调度</div></div>
      <div class="limit"><span class="metric-label">请求限流</span><strong>60 / 分</strong><div class="metric-note">独立规则</div></div>
      <div class="limit"><span class="metric-label">周期额度</span><strong>35%</strong><div class="metric-note">已使用 12%</div></div>
    </div>
    <div class="share-footer">
      <div><div class="account-name">访问控制</div><div class="account-meta">共享端无法修改本机设置</div></div>
      <div class="button-row"><span class="button">暂停</span><span class="button">撤销</span><span class="button primary">管理共享</span></div>
    </div>
  </div>
</section>
`;

const codexContent = String.raw`
<div class="segmented codex-tabs">
  <span class="segment">本地注入</span>
  <span class="segment active">远程注入</span>
</div>
<section class="connection-panel">
  <div class="connection-head">
    <div><div class="connection-title">Codex 一键接入</div><div class="connection-copy">先校验连接，再通过反向隧道把远程请求安全回流到本地账号池</div></div>
    <div class="button-row"><span class="button">测试连接</span><span class="button primary">⚡ 一键注入</span></div>
  </div>
  <div class="flow">
    <article class="flow-node active"><div class="flow-icon">A</div><div class="flow-title">Amber 账号池</div><div class="flow-copy">额度感知 · 自动切换</div></article>
    <div class="flow-arrow">→</div>
    <article class="flow-node teal-node"><div class="flow-icon">↔</div><div class="flow-title">SSH 反向隧道</div><div class="flow-copy">连接已校验 · 路由受控</div></article>
    <div class="flow-arrow">→</div>
    <article class="flow-node"><div class="flow-icon">&gt;_</div><div class="flow-title">Codex CLI</div><div class="flow-copy">配置自动写入与备份</div></article>
  </div>
  <div class="config-grid">
    <div class="config-cell"><span class="metric-label">连接方式</span><strong>反向隧道回流本机</strong></div>
    <div class="config-cell"><span class="metric-label">配置文件</span><strong>config.toml · auth.json</strong></div>
    <div class="config-cell"><span class="metric-label">模型策略</span><strong>跟随账号池设置</strong></div>
  </div>
</section>
<section class="injection-status">
  <div><strong>✓ 远程 Codex 已接入</strong><p>原配置已自动备份，断开后可一键恢复。</p></div>
  <div class="chip-row"><span class="chip teal">隧道正常</span><span class="chip">自动恢复路由</span><span class="button">重新注入</span></div>
</section>
`;

const scenes = [
  {
    file: 'dashboard-v044.png',
    width: 1440,
    height: 900,
    scale: 2,
    bodyClass: 'desktop',
    scene: {
      active: 'dashboard',
      title: '仪表盘',
      subtitle: '统一查看账号池、调用状态与 Codex 接入',
      actions: '<span class="button primary">▶ 服务运行中</span>',
      content: dashboardContent,
    },
  },
  {
    file: 'dashboard-v044-mobile.png',
    width: 540,
    height: 675,
    scale: 2,
    bodyClass: 'mobile',
    scene: {
      active: 'dashboard',
      title: '仪表盘',
      subtitle: '账号池与 Codex 网关运行概览',
      actions: '',
      content: dashboardContent,
    },
  },
  {
    file: 'hero-cover-v044.png',
    width: 1440,
    height: 450,
    scale: 2,
    bodyClass: 'desktop hero-cover',
    scene: {
      active: 'dashboard',
      title: '仪表盘',
      subtitle: '统一查看账号池、调用状态与 Codex 接入',
      actions: '<span class="button primary">▶ 服务运行中</span>',
      content: dashboardContent,
    },
  },
  {
    file: 'hero-cover-v044-mobile.png',
    width: 540,
    height: 360,
    scale: 2,
    bodyClass: 'mobile hero-cover',
    scene: {
      active: 'dashboard',
      title: '仪表盘',
      subtitle: '账号池与 Codex 网关运行概览',
      actions: '',
      content: dashboardContent,
    },
  },
  {
    file: 'accounts-v044.png',
    width: 1440,
    height: 900,
    scale: 2,
    bodyClass: 'desktop',
    scene: {
      active: 'accounts',
      title: '账号调度',
      subtitle: '额度感知、并发队列、批量测试与自动切换',
      actions: '',
      content: accountsContent,
    },
  },
  {
    file: 'network-v044.png',
    width: 1440,
    height: 900,
    scale: 2,
    bodyClass: 'desktop',
    scene: {
      active: 'network',
      title: '代理与网络出口',
      subtitle: '为账号配置独立网络策略，并在异常时自动回退',
      actions: '',
      content: networkContent,
    },
  },
  {
    file: 'cloud-sharing-v044.png',
    width: 1440,
    height: 900,
    scale: 2,
    bodyClass: 'desktop',
    scene: {
      active: 'sharing',
      title: '云账户与共享',
      subtitle: '加密同步个人配置，把少量账号安全共享给固定好友',
      actions: '<span class="button teal">＋ 新建共享</span>',
      content: sharingContent,
    },
  },
  {
    file: 'codex-injection-v044.png',
    width: 1440,
    height: 900,
    scale: 2,
    bodyClass: 'desktop',
    scene: {
      active: 'codex',
      title: 'Codex 接入',
      subtitle: '本地启动并注入，或通过受控隧道接入远程 Codex',
      actions: '',
      content: codexContent,
    },
  },
];

const compactSceneFiles = new Set([
  'accounts-v044.png',
  'network-v044.png',
  'cloud-sharing-v044.png',
  'codex-injection-v044.png',
]);

const renderScenes = [
  ...scenes,
  ...scenes
    .filter((scene) => compactSceneFiles.has(scene.file))
    .map((scene) => ({ ...scene, file: scene.file.replace('.png', '-compact.png'), scale: 1 })),
];

const ogStyles = String.raw`
  :root {
    font-family: "Segoe UI", "Microsoft YaHei UI", "Microsoft YaHei", Arial, sans-serif;
    color: #202522;
  }

  * {
    box-sizing: border-box;
  }

  html,
  body {
    width: 100%;
    height: 100%;
    margin: 0;
  }

  body {
    overflow: hidden;
    background: #f3f3ef;
    letter-spacing: 0;
  }

  .og {
    position: relative;
    display: grid;
    width: 600px;
    height: 315px;
    grid-template-columns: 0.92fr 1.08fr;
    overflow: hidden;
    border: 1px solid #d6d7d1;
    background: #f7f7f4;
  }

  .copy {
    display: flex;
    flex-direction: column;
    justify-content: center;
    padding: 34px 22px 34px 38px;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 22px;
  }

  .brand img {
    width: 38px;
    height: 38px;
  }

  .brand strong {
    font-size: 20px;
  }

  h1 {
    max-width: 250px;
    margin: 0;
    font-size: 27px;
    line-height: 1.16;
    font-weight: 780;
  }

  h1 span {
    color: #c35231;
  }

  p {
    max-width: 235px;
    margin: 14px 0 0;
    color: #686d67;
    font-size: 10px;
    line-height: 1.65;
  }

  .meta {
    display: flex;
    gap: 6px;
    margin-top: 19px;
  }

  .tag {
    border: 1px solid #d9d9d2;
    border-radius: 999px;
    background: #fff;
    padding: 5px 8px;
    color: #565b56;
    font-size: 7px;
    font-weight: 700;
  }

  .product-wrap {
    display: flex;
    align-items: center;
    padding: 27px 30px 27px 4px;
  }

  .product {
    width: 100%;
    overflow: hidden;
    border: 1px solid #d2d3cd;
    border-radius: 7px;
    background: #fff;
    box-shadow: 0 18px 34px rgba(31, 36, 32, 0.14);
  }

  .titlebar {
    display: flex;
    height: 30px;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid #e1e1db;
    background: #eeece5;
    padding: 0 10px;
    font-size: 7px;
    font-weight: 700;
  }

  .titlebar-left {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .titlebar img {
    width: 16px;
    height: 16px;
  }

  .demo {
    color: #8a5842;
  }

  .product-body {
    padding: 12px;
  }

  .status {
    display: flex;
    height: 32px;
    align-items: center;
    justify-content: space-between;
    border: 1px solid #b9dcd4;
    border-radius: 5px;
    background: #e3f2ef;
    padding: 0 9px;
    color: #155f59;
    font-size: 7px;
    font-weight: 700;
  }

  .metrics {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 6px;
    margin: 8px 0;
  }

  .metric {
    min-height: 45px;
    border: 1px solid #deded8;
    border-radius: 5px;
    padding: 8px;
    color: #737770;
    font-size: 6px;
  }

  .metric strong {
    display: block;
    margin-top: 6px;
    color: #202522;
    font-size: 13px;
  }

  .rows {
    overflow: hidden;
    border: 1px solid #deded8;
    border-radius: 5px;
  }

  .row {
    display: grid;
    height: 34px;
    grid-template-columns: 1fr 48px 78px;
    align-items: center;
    gap: 7px;
    border-bottom: 1px solid #e8e8e2;
    padding: 0 9px;
    font-size: 7px;
    font-weight: 700;
  }

  .row:last-child {
    border-bottom: 0;
  }

  .pill {
    border-radius: 999px;
    background: #e5f3eb;
    color: #287a52;
    padding: 4px 6px;
    text-align: center;
    font-size: 6px;
  }

  .bar {
    height: 4px;
    overflow: hidden;
    border-radius: 999px;
    background: #e9e8e2;
  }

  .bar span {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: #d8643f;
  }
`;

const ogBody = String.raw`
<div class="og">
  <section class="copy">
    <div class="brand"><img src="__MARK__" alt=""><strong>Amber</strong></div>
    <h1>把多个账号<br>变成<span>可调度、</span><br><span>可共享</span>的<br>Codex 网关</h1>
    <p>Windows 上统一管理账号、代理、加密云同步和本地/远程 Codex 接入。</p>
    <div class="meta"><span class="tag">Windows x64</span><span class="tag">本地优先</span><span class="tag">v0.4.4</span></div>
  </section>
  <section class="product-wrap">
    <div class="product">
      <div class="titlebar"><div class="titlebar-left"><img src="__MARK__" alt="">Amber 工作台</div><span class="demo">演示数据 · v0.4.4</span></div>
      <div class="product-body">
        <div class="status"><span>● 本地网关运行中</span><span>自动调度</span></div>
        <div class="metrics"><div class="metric">可用账号<strong>3</strong></div><div class="metric">今日请求<strong>248</strong></div><div class="metric">额度使用<strong>37%</strong></div></div>
        <div class="rows">
          <div class="row"><span>主力额度</span><span class="pill">正常</span><span class="bar"><span style="width:28%"></span></span></div>
          <div class="row"><span>备用额度</span><span class="pill">正常</span><span class="bar"><span style="width:16%"></span></span></div>
          <div class="row"><span>共享额度</span><span class="pill">待命</span><span class="bar"><span style="width:42%"></span></span></div>
        </div>
      </div>
    </div>
  </section>
</div>
`;

function ogDocument() {
  return pageTemplate
    .replace('__STYLES__', ogStyles)
    .replace('__BODY_CLASS__', '')
    .replace('__BODY__', ogBody.replaceAll('__MARK__', markUri));
}

function assertSafeMarkup(markup, label) {
  const visible = markup
    .replace(/<style[\s\S]*?<\/style>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ');

  const forbidden = [
    [/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i, 'email'],
    [/\b(?:\d{1,3}\.){3}\d{1,3}\b/, 'IP address'],
    [/https?:\/\//i, 'URL'],
    [/\bacct[_-]/i, 'account identifier'],
    [/\b(?:api|guest)[ _-]?key\b/i, 'key label'],
    [/\bC:\\Users\\/i, 'local user path'],
    [/(?:用户名|账号\s*(?:ID|编号)|指纹|设备名|主机[:：])/i, 'identity field'],
    [/\.(?:example|test)\b/i, 'example host'],
  ];

  for (const [pattern, description] of forbidden) {
    if (pattern.test(visible)) {
      throw new Error(label + ' contains forbidden ' + description + ' text.');
    }
  }
}

function pngDimensions(buffer) {
  if (buffer.toString('ascii', 1, 4) !== 'PNG') {
    throw new Error('Expected PNG output.');
  }
  return {
    width: buffer.readUInt32BE(16),
    height: buffer.readUInt32BE(20),
  };
}

async function render(browser, target) {
  const context = await browser.newContext({
    viewport: { width: target.width, height: target.height },
    deviceScaleFactor: target.scale,
    colorScheme: 'light',
  });
  const page = await context.newPage();
  const markup = target.markup;
  assertSafeMarkup(markup, target.file);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.setContent(markup, { waitUntil: 'load' });
  await page.evaluate(async () => {
    await document.fonts.ready;
  });
  const buffer = await page.screenshot({
    path: target.path,
    type: 'png',
    animations: 'disabled',
    fullPage: false,
  });
  await context.close();

  const dimensions = pngDimensions(buffer);
  const expectedWidth = target.width * target.scale;
  const expectedHeight = target.height * target.scale;
  if (dimensions.width !== expectedWidth || dimensions.height !== expectedHeight) {
    throw new Error(
      target.file +
        ' has dimensions ' +
        dimensions.width +
        'x' +
        dimensions.height +
        ', expected ' +
        expectedWidth +
        'x' +
        expectedHeight +
        '.',
    );
  }

  return {
    file: target.file,
    dimensions: dimensions.width + 'x' + dimensions.height,
    bytes: buffer.length,
  };
}

await mkdir(outputDirectory, { recursive: true });

const browser = await chromium.launch({ headless: true });
const results = [];

try {
  for (const target of renderScenes) {
    const body = shell(target.scene);
    results.push(
      await render(browser, {
        ...target,
        path: path.join(outputDirectory, target.file),
        markup: documentFor(body, target.bodyClass),
      }),
    );
  }

  results.push(
    await render(browser, {
      file: 'og-cover-v044.png',
      width: 600,
      height: 315,
      scale: 2,
      path: ogPath,
      markup: ogDocument(),
    }),
  );
} finally {
  await browser.close();
}

console.table(results);
