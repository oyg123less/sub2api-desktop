<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  api,
  type Account,
  type CloudDevice,
  type CloudFriend,
  type CloudFriendRequest,
  type CloudProfile,
  type CloudReceivedShare,
  type CloudShareGroup,
  type CloudShareGroupAccount,
  type CloudShareGroupDetails,
  type CloudShareGroupRecipient,
  type CloudShareGroupUsage,
  type CloudStatus,
  type CreateShareGroupInput,
} from "../../api/control";
import { useAppStore } from "../../store";
import ConfirmModal from "../ConfirmModal.vue";
import Icon from "../Icon.vue";
import {
  clearWorkspaceCache,
  readWorkspaceCache,
  writeWorkspaceCache,
  type CloudWorkspaceSnapshot,
} from "./workspaceCache";

const props = defineProps<{ status: CloudStatus; busy: string; adminOpen: boolean }>();
const emit = defineEmits<{ sync: []; network: []; logout: []; admin: []; password: [] }>();
const { t } = useI18n();
const app = useAppStore();

type CloudTab = "overview" | "shares" | "received" | "friends" | "devices" | "security";
const tab = ref<CloudTab>("overview");
const loading = ref(true);
const actionBusy = ref("");
const loadError = ref("");
const profile = ref<CloudProfile | null>(null);
const friends = ref<CloudFriend[]>([]);
const friendRequests = ref<CloudFriendRequest[]>([]);
const shareGroups = ref<CloudShareGroup[]>([]);
const receivedShares = ref<CloudReceivedShare[]>([]);
const devices = ref<CloudDevice[]>([]);
const accounts = ref<Account[]>([]);
const relayEnabled = ref(false);

const addFriendOpen = ref(false);
const friendCodeInput = ref("");
const wizardOpen = ref(false);
const wizardStep = ref(1);
const wizard = ref(newWizard());
const detailOpen = ref(false);
const detail = ref<CloudShareGroupDetails | null>(null);
const detailTab = ref<"accounts" | "friends" | "usage">("accounts");
const detailUsage = ref<CloudShareGroupUsage[]>([]);
const detailUsageLoading = ref(false);
const deleteGroupTarget = ref<CloudShareGroup | null>(null);
const revealKeys = ref<Record<string, boolean>>({});
const wizardSearch = ref("");
const accountPickerOpen = ref(false);
const accountPickerID = ref<number | null>(null);
const accountPickerRelay = ref<"owner_device" | "worker_direct">("owner_device");
const invitePickerOpen = ref(false);
const inviteFriendIDs = ref<string[]>([]);
const recipientEditor = ref<CloudShareGroupRecipient | null>(null);
const recipientRules = ref({ rpm_limit: 30, concurrency_limit: 2, quota_requests: 0, expires_at: "" });

function newIdempotencyKey() {
  return globalThis.crypto?.randomUUID?.() || `amber-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function newWizard(): CreateShareGroupInput {
  return {
    idempotency_key: newIdempotencyKey(),
    name: "",
    description: "",
    route_policy: "balanced",
    default_rpm: 30,
    default_concurrency: 2,
    default_quota_requests: 0,
    default_expires_at: "",
    accounts: [],
    recipients: [],
  };
}

const incomingRequests = computed(() => friendRequests.value.filter((request) => request.direction === "incoming" && request.status === "pending"));
const outgoingRequests = computed(() => friendRequests.value.filter((request) => request.direction === "outgoing" && request.status === "pending"));
const pendingReceived = computed(() => receivedShares.value.filter((share) => share.status === "pending"));
const activeGroups = computed(() => shareGroups.value.filter((group) => group.status === "active"));
const activeReceived = computed(() => receivedShares.value.filter((share) => share.status === "active" || share.status === "paused"));
const onlineDevices = computed(() => devices.value.filter((device) => device.online && !device.revoked));
const retrySeconds = computed(() => {
  if (!props.status.next_retry_at) return 0;
  return Math.max(0, Math.ceil((new Date(props.status.next_retry_at).getTime() - Date.now()) / 1000));
});
const syncErrorMessage = computed(() => props.status.last_error_stage
  ? t(`cloud.syncErrorStage.${props.status.last_error_stage}`)
  : props.status.last_error || "");
const filteredWizardAccounts = computed(() => {
  const query = wizardSearch.value.trim().toLowerCase();
  return query ? accounts.value.filter((account) => accountName(account).toLowerCase().includes(query)) : accounts.value;
});
const filteredWizardFriends = computed(() => {
  const query = wizardSearch.value.trim().toLowerCase();
  return query ? friends.value.filter((friend) => `${friend.alias || ""} ${friend.display_name} ${friend.friend_code}`.toLowerCase().includes(query)) : friends.value;
});
const availableDetailAccounts = computed(() => {
  const existing = new Set((detail.value?.accounts || []).map((account) => account.account_uid));
  return accounts.value.filter((account) => account.status === "active" && account.client_uid && !existing.has(account.client_uid));
});
const availableDetailFriends = computed(() => {
  const existing = new Set((detail.value?.recipients || []).filter((recipient) => recipient.status !== "revoked").map((recipient) => recipient.friendship_id));
  return friends.value.filter((friend) => !existing.has(friend.public_id));
});

const tabs = computed(() => [
  { id: "overview" as const, label: t("cloud.v4.tabs.overview"), count: 0 },
  { id: "shares" as const, label: t("cloud.v4.tabs.shares"), count: 0 },
  { id: "received" as const, label: t("cloud.v4.tabs.received"), count: pendingReceived.value.length },
  { id: "friends" as const, label: t("cloud.v4.tabs.friends"), count: incomingRequests.value.length },
  { id: "devices" as const, label: t("cloud.v4.tabs.devices"), count: 0 },
  { id: "security" as const, label: t("cloud.v4.tabs.security"), count: 0 },
]);

function applyWorkspace(snapshot: CloudWorkspaceSnapshot) {
  profile.value = snapshot.profile;
  friends.value = snapshot.friends;
  friendRequests.value = snapshot.friendRequests;
  shareGroups.value = snapshot.shareGroups;
  receivedShares.value = snapshot.receivedShares;
  devices.value = snapshot.devices;
  accounts.value = snapshot.accounts;
  relayEnabled.value = snapshot.relayEnabled;
}

async function refreshWorkspace(silent: boolean) {
  if (!silent) loading.value = true;
  if (!silent) loadError.value = "";
  try {
    const [workspace, accountsResult] = await Promise.all([
      api.cloudWorkspace(),
      api.listAccounts(),
    ]);
    const snapshot: CloudWorkspaceSnapshot = {
      profile: workspace.profile,
      friends: workspace.friends?.friends || [],
      friendRequests: workspace.friend_requests?.requests || [],
      shareGroups: workspace.share_groups?.groups || [],
      receivedShares: workspace.received_shares?.shares || [],
      devices: workspace.devices?.devices || [],
      accounts: accountsResult.accounts || [],
      relayEnabled: Boolean(workspace.devices?.relay_enabled),
    };
    applyWorkspace(snapshot);
    writeWorkspaceCache(props.status.email || "", snapshot);
    loadError.value = "";
  } catch (error) {
    if (!silent || !profile.value) {
      loadError.value = (error as Error).message;
      app.toast(loadError.value, "error");
    }
  } finally {
    if (!silent) loading.value = false;
  }
}

async function loadWorkspace(silent = false) {
  if (!silent) {
    const cached = readWorkspaceCache(props.status.email || "");
    if (cached) {
      applyWorkspace(cached);
      loading.value = false;
      loadError.value = "";
      void refreshWorkspace(true);
      return;
    }
  }
  await refreshWorkspace(silent);
}

function logoutWorkspace() {
  clearWorkspaceCache(props.status.email || "");
  emit("logout");
}

function fmt(value?: string) {
  if (!value) return t("cloud.never");
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function accountName(account: Account) {
  return account.email || account.chatgpt_account_id || account.base_url || `#${account.id}`;
}

function selectedAccount(accountID: number) {
  return wizard.value.accounts.some((item) => item.account_id === accountID);
}

function toggleWizardAccount(account: Account) {
  const index = wizard.value.accounts.findIndex((item) => item.account_id === account.id);
  if (index >= 0) wizard.value.accounts.splice(index, 1);
  else wizard.value.accounts.push({ account_id: account.id, relay_mode: "owner_device", priority: 100, weight: 100 });
}

function setWizardRelay(accountID: number, relayMode: "owner_device" | "worker_direct") {
  const selection = wizard.value.accounts.find((item) => item.account_id === accountID);
  if (selection) selection.relay_mode = relayMode;
}

function selectedFriend(friendID: string) {
  return wizard.value.recipients.some((item) => item.friendship_id === friendID);
}

function toggleWizardFriend(friend: CloudFriend) {
  const index = wizard.value.recipients.findIndex((item) => item.friendship_id === friend.public_id);
  if (index >= 0) wizard.value.recipients.splice(index, 1);
  else wizard.value.recipients.push({ friendship_id: friend.public_id });
}

function openWizard() {
  wizard.value = newWizard();
  wizardStep.value = 1;
  wizardSearch.value = "";
  wizardOpen.value = true;
}

function closeWizard() {
  if (actionBusy.value) return;
  wizardOpen.value = false;
}

function nextWizardStep() {
  if (wizardStep.value === 1 && wizard.value.name.trim().length < 2) {
    app.toast(t("cloud.v4.wizard.nameRequired"), "error");
    return;
  }
  if (wizardStep.value === 2 && wizard.value.accounts.length === 0) {
    app.toast(t("cloud.v4.wizard.accountRequired"), "error");
    return;
  }
  if (wizardStep.value === 3 && wizard.value.recipients.length === 0) {
    app.toast(t("cloud.v4.wizard.friendRequired"), "error");
    return;
  }
  wizardStep.value = Math.min(5, wizardStep.value + 1);
}

async function createShareGroup() {
  actionBusy.value = "create-group";
  try {
    const created = await api.cloudCreateShareGroup({ ...wizard.value, name: wizard.value.name.trim(), description: wizard.value.description.trim() });
    app.toast(t("cloud.v4.shareCreated"), "success");
    wizardOpen.value = false;
    await loadWorkspace(true);
    tab.value = "shares";
    detail.value = created;
    detailTab.value = "accounts";
    detailOpen.value = true;
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function openGroup(group: CloudShareGroup) {
  actionBusy.value = `detail-${group.public_id}`;
  try {
    detail.value = await api.cloudShareGroup(group.public_id);
    detailTab.value = "accounts";
    detailOpen.value = true;
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function refreshDetail() {
  if (!detail.value) return;
  detail.value = await api.cloudShareGroup(detail.value.group.public_id);
}

async function loadDetailUsage() {
  if (!detail.value || detailUsageLoading.value) return;
  detailUsageLoading.value = true;
  try {
    const result = await api.cloudShareGroupUsage(detail.value.group.public_id);
    detailUsage.value = result.usage || [];
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    detailUsageLoading.value = false;
  }
}

async function toggleGroup(group: CloudShareGroup) {
  actionBusy.value = `group-${group.public_id}`;
  try {
    await api.cloudUpdateShareGroup(group.public_id, { status: group.status === "active" ? "paused" : "active" });
    await loadWorkspace(true);
    if (detail.value?.group.public_id === group.public_id) detail.value = await api.cloudShareGroup(group.public_id);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function deleteGroup() {
  const target = deleteGroupTarget.value;
  if (!target) return;
  actionBusy.value = "delete-group";
  try {
    await api.cloudDeleteShareGroup(target.public_id);
    deleteGroupTarget.value = null;
    detailOpen.value = false;
    await loadWorkspace(true);
    app.toast(t("cloud.v4.shareDeleted"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function toggleRecipient(recipient: CloudShareGroupRecipient) {
  if (!detail.value) return;
  actionBusy.value = `recipient-${recipient.public_id}`;
  try {
    await api.cloudUpdateShareGroupRecipient(detail.value.group.public_id, recipient.public_id, { status: recipient.status === "active" ? "paused" : "active" });
    detail.value = await api.cloudShareGroup(detail.value.group.public_id);
    await loadWorkspace(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function rotateRecipient(recipient: CloudShareGroupRecipient) {
  if (!detail.value || !recipient.friendship_id) return;
  actionBusy.value = `rotate-${recipient.public_id}`;
  try {
    await api.cloudRotateShareGroupKey(detail.value.group.public_id, recipient.public_id, recipient.friendship_id, newIdempotencyKey());
    detail.value = await api.cloudShareGroup(detail.value.group.public_id);
    app.toast(t("cloud.v4.keyRotated"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function revokeRecipient(recipient: CloudShareGroupRecipient) {
  if (!detail.value) return;
  actionBusy.value = `revoke-${recipient.public_id}`;
  try {
    await api.cloudDeleteShareGroupRecipient(detail.value.group.public_id, recipient.public_id);
    detail.value = await api.cloudShareGroup(detail.value.group.public_id);
    await loadWorkspace(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function toggleGroupAccount(accountId: string, enabled: boolean | number) {
  if (!detail.value) return;
  actionBusy.value = `account-${accountId}`;
  try {
    await api.cloudUpdateShareGroupAccount(detail.value.group.public_id, accountId, { enabled: !Boolean(enabled) });
    detail.value = await api.cloudShareGroup(detail.value.group.public_id);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

function openAccountPicker() {
  accountPickerID.value = availableDetailAccounts.value[0]?.id || null;
  accountPickerRelay.value = "owner_device";
  accountPickerOpen.value = true;
}

function selectedPickerAccount(): Account | undefined {
  return accounts.value.find((account) => account.id === accountPickerID.value);
}

async function addGroupAccount() {
  if (!detail.value || !accountPickerID.value) return;
  const account = selectedPickerAccount();
  const relayMode = account?.account_type === "oauth" ? "owner_device" : accountPickerRelay.value;
  actionBusy.value = "add-group-account";
  try {
    await api.cloudAddShareGroupAccount(detail.value.group.public_id, { account_id: accountPickerID.value, relay_mode: relayMode });
    accountPickerOpen.value = false;
    await refreshDetail();
    await loadWorkspace(true);
    app.toast(t("cloud.v4.accountAdded"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function removeGroupAccount(accountId: string) {
  if (!detail.value) return;
  actionBusy.value = `remove-account-${accountId}`;
  try {
    await api.cloudDeleteShareGroupAccount(detail.value.group.public_id, accountId);
    detail.value = await api.cloudShareGroup(detail.value.group.public_id);
    await loadWorkspace(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

function openInvitePicker() {
  inviteFriendIDs.value = [];
  invitePickerOpen.value = true;
}

function toggleInviteFriend(friendID: string) {
  const index = inviteFriendIDs.value.indexOf(friendID);
  if (index >= 0) inviteFriendIDs.value.splice(index, 1);
  else inviteFriendIDs.value.push(friendID);
}

async function inviteGroupFriends() {
  if (!detail.value || !inviteFriendIDs.value.length) return;
  actionBusy.value = "invite-group-friends";
  try {
    await api.cloudInviteShareGroupRecipients(detail.value.group.public_id, {
      idempotency_key: newIdempotencyKey(),
      recipients: inviteFriendIDs.value.map((friendship_id) => ({ friendship_id })),
    });
    invitePickerOpen.value = false;
    await refreshDetail();
    await loadWorkspace(true);
    app.toast(t("cloud.v4.friendsInvited"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

function editRecipient(recipient: CloudShareGroupRecipient) {
  recipientEditor.value = recipient;
  recipientRules.value = {
    rpm_limit: recipient.rpm_limit,
    concurrency_limit: recipient.concurrency_limit,
    quota_requests: recipient.quota_requests,
    expires_at: recipient.expires_at ? recipient.expires_at.slice(0, 10) : "",
  };
}

async function saveRecipientRules() {
  if (!detail.value || !recipientEditor.value) return;
  actionBusy.value = "recipient-rules";
  try {
    await api.cloudUpdateShareGroupRecipient(detail.value.group.public_id, recipientEditor.value.public_id, { ...recipientRules.value });
    recipientEditor.value = null;
    await refreshDetail();
    app.toast(t("cloud.v4.rulesSaved"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function sendFriendRequest() {
  actionBusy.value = "add-friend";
  try {
    await api.cloudAddFriend(friendCodeInput.value.trim().toUpperCase());
    friendCodeInput.value = "";
    addFriendOpen.value = false;
    await loadWorkspace(true);
    app.toast(t("cloud.v4.friendRequestSent"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function friendRequestAction(request: CloudFriendRequest, action: "accept" | "decline" | "cancel") {
  actionBusy.value = `friend-${request.public_id}`;
  try {
    await api.cloudFriendRequestAction(request.public_id, action);
    await loadWorkspace(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function removeFriend(friend: CloudFriend) {
  actionBusy.value = `friend-remove-${friend.public_id}`;
  try {
    await api.cloudDeleteFriend(friend.public_id, false);
    await loadWorkspace(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function receivedAction(share: CloudReceivedShare, action: "accept" | "decline" | "leave") {
  actionBusy.value = `received-${share.public_id}`;
  try {
    await api.cloudReceivedShareAction(share.public_id, action);
    await loadWorkspace(true);
    app.toast(t(action === "accept" ? "cloud.v4.shareAccepted" : action === "decline" ? "cloud.v4.shareDeclined" : "cloud.v4.shareLeft"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function testReceivedShare(share: CloudReceivedShare) {
  actionBusy.value = `test-${share.public_id}`;
  try {
    const result = await api.cloudTestReceivedShare(share.public_id);
    app.toast(result.ok ? t("cloud.v4.testShareSuccess") : `${result.code || result.status}: ${result.message}`, result.ok ? "success" : "error");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function ensureDevice() {
  actionBusy.value = "ensure-device";
  try {
    await api.cloudEnsureDevice();
    await loadWorkspace(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function toggleRelay() {
  actionBusy.value = "relay";
  try {
    relayEnabled.value = !relayEnabled.value;
    await api.cloudSetRelay(relayEnabled.value);
    await loadWorkspace(true);
  } catch (error) {
    relayEnabled.value = !relayEnabled.value;
    app.toast((error as Error).message, "error");
  } finally {
    actionBusy.value = "";
  }
}

async function copy(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    app.toast(t("common.copied"), "success");
  } catch {
    app.toast(t("common.copyFailed"), "error");
  }
}

onMounted(() => loadWorkspace());
watch(() => props.status.last_sync_at, (current, previous) => {
  if (current && current !== previous) void loadWorkspace(true);
});
watch(detailTab, (current) => {
  if (current === "usage") void loadDetailUsage();
});
</script>

<template>
  <section class="cloud-workspace" data-test="cloud-v4-workspace">
    <header class="workspace-header">
      <div class="workspace-identity">
        <span class="workspace-avatar">{{ (profile?.display_name || status.email || "A").slice(0, 1).toUpperCase() }}</span>
        <div>
          <h1>{{ profile?.display_name || status.email }}</h1>
          <p>{{ status.email }} · {{ t(status.role === "admin" ? "cloud.roleAdmin" : "cloud.roleUser") }}</p>
        </div>
      </div>
      <div class="workspace-actions">
        <span class="relay-state" :class="onlineDevices.length ? 'online' : 'offline'"><i></i>{{ onlineDevices.length ? t("cloud.v4.relayOnline") : t("cloud.v4.relayOffline") }}</span>
        <button class="btn btn-ghost" data-test="cloud-sync" type="button" :disabled="busy !== ''" @click="emit('sync')"><Icon name="refresh" :size="14" />{{ busy === "sync" ? t("cloud.syncing") : t("cloud.syncNow") }}</button>
        <button v-if="status.role === 'admin'" class="btn btn-ghost" data-test="cloud-admin-open" type="button" @click="emit('admin')"><Icon name="settings" :size="14" />{{ t("cloud.adminOpen") }}</button>
        <button class="btn btn-ghost icon-only" data-test="cloud-logout" type="button" :title="t('cloud.logout')" :disabled="busy !== ''" @click="logoutWorkspace"><Icon name="power" :size="16" /></button>
      </div>
    </header>

    <nav class="workspace-tabs" role="tablist" :aria-label="t('cloud.title')">
      <button v-for="item in tabs" :key="item.id" role="tab" type="button" :data-test="`cloud-tab-${item.id}`" :aria-selected="tab === item.id" :class="{ active: tab === item.id }" @click="tab = item.id">
        {{ item.label }}<span v-if="item.count" class="tab-count">{{ item.count > 99 ? "99+" : item.count }}</span>
      </button>
    </nav>

    <section v-if="status.last_error" class="sync-error-band" role="alert">
      <Icon name="warn" :size="18" />
      <div><strong>{{ t("cloud.syncFailed") }}</strong><p>{{ syncErrorMessage }}</p><span v-if="status.last_attempt_at">{{ t("cloud.syncAttempt", { time: fmt(status.last_attempt_at) }) }}</span><span v-if="status.consecutive_failures">{{ t("cloud.syncFailures", { count: status.consecutive_failures }) }}</span><span v-if="retrySeconds">{{ t("cloud.syncRetryIn", { seconds: retrySeconds }) }}</span></div>
      <div class="sync-error-actions">
        <button class="btn btn-ghost btn-sm" data-test="cloud-network-open" type="button" :disabled="busy !== ''" @click="emit('network')"><Icon name="proxies" :size="14" />{{ t("cloud.networkSettings") }}</button>
        <button class="btn btn-ghost btn-sm" data-test="cloud-sync-retry" type="button" :disabled="busy !== ''" @click="emit('sync')"><Icon name="refresh" :size="14" />{{ t("cloud.syncRetry") }}</button>
      </div>
    </section>

    <div v-if="loading" class="workspace-loading"><span></span><span></span><span></span></div>
    <div v-else-if="loadError" class="workspace-error" role="alert"><Icon name="warn" :size="20" /><span>{{ loadError }}</span><button class="btn btn-ghost btn-sm" @click="loadWorkspace()">{{ t("common.retry") }}</button></div>

    <div v-else class="workspace-content">
      <template v-if="tab === 'overview'">
        <section class="metric-strip" :aria-label="t('cloud.v4.tabs.overview')">
          <div><span>{{ t("cloud.v4.metrics.activeShares") }}</span><strong>{{ activeGroups.length }}</strong></div>
          <div><span>{{ t("cloud.v4.metrics.relayDevices") }}</span><strong>{{ onlineDevices.length }}</strong></div>
          <div><span>{{ t("cloud.v4.metrics.received") }}</span><strong>{{ activeReceived.length }}</strong></div>
          <div><span>{{ t("cloud.lastSync") }}</span><strong class="metric-time">{{ fmt(status.last_sync_at) }}</strong></div>
        </section>

        <section class="workspace-section">
          <div class="section-title"><div><h2>{{ t("cloud.v4.pendingTitle") }}</h2><p>{{ t("cloud.v4.pendingDesc") }}</p></div></div>
          <div v-if="!incomingRequests.length && !pendingReceived.length && !status.last_error" class="plain-empty"><Icon name="check" :size="22" /><span>{{ t("cloud.v4.noPending") }}</span></div>
          <div v-else class="task-list">
            <button v-for="request in incomingRequests" :key="request.public_id" type="button" @click="tab = 'friends'"><span class="task-icon"><Icon name="accounts" :size="16" /></span><span><strong>{{ request.display_name }}</strong>{{ t("cloud.v4.friendRequestTask") }}</span><Icon name="external" :size="14" /></button>
            <button v-for="share in pendingReceived" :key="share.public_id" type="button" @click="tab = 'received'"><span class="task-icon"><Icon name="link" :size="16" /></span><span><strong>{{ share.group.name }}</strong>{{ t("cloud.v4.shareInviteTask", { name: share.owner.display_name }) }}</span><Icon name="external" :size="14" /></button>
            <button v-if="status.last_error" type="button" @click="emit('sync')"><span class="task-icon danger"><Icon name="warn" :size="16" /></span><span><strong>{{ t("cloud.syncFailed") }}</strong>{{ status.last_error }}</span><Icon name="refresh" :size="14" /></button>
          </div>
        </section>

        <section class="workspace-section overview-columns">
          <div>
            <div class="section-title"><h2>{{ t("cloud.v4.mySharesTitle") }}</h2><button class="text-action" @click="tab = 'shares'">{{ t("common.view") }}</button></div>
            <div v-if="shareGroups.length" class="compact-list"><button v-for="group in shareGroups.slice(0, 4)" :key="group.public_id" @click="openGroup(group)"><span class="status-dot" :class="group.status"></span><span><strong>{{ group.name }}</strong><small>{{ t("cloud.v4.groupCounts", { accounts: group.account_count, friends: group.recipient_count }) }}</small></span><span class="faint">{{ group.used_requests || 0 }}</span></button></div>
            <div v-else class="plain-empty compact">{{ t("cloud.v4.noShares") }}</div>
          </div>
          <div>
            <div class="section-title"><h2>{{ t("cloud.v4.friendsTitle") }}</h2><button class="text-action" @click="tab = 'friends'">{{ t("common.view") }}</button></div>
            <div v-if="friends.length" class="compact-list"><button v-for="friend in friends.slice(0, 4)" :key="friend.public_id" @click="tab = 'friends'"><span class="friend-avatar">{{ friend.display_name.slice(0, 1).toUpperCase() }}</span><span><strong>{{ friend.alias || friend.display_name }}</strong><small class="mono">{{ friend.friend_code }}</small></span></button></div>
            <div v-else class="plain-empty compact">{{ t("cloud.v4.noFriends") }}</div>
          </div>
        </section>
      </template>

      <template v-else-if="tab === 'shares'">
        <section class="section-title page-section-title"><div><h2>{{ t("cloud.v4.mySharesTitle") }}</h2><p>{{ t("cloud.v4.mySharesDesc") }}</p></div><button class="btn btn-primary" data-test="new-share-group" @click="openWizard"><Icon name="plus" :size="15" />{{ t("cloud.v4.newShare") }}</button></section>
        <div v-if="!shareGroups.length" class="feature-empty"><Icon name="link" :size="28" /><h3>{{ t("cloud.v4.noShares") }}</h3><p>{{ t("cloud.v4.noSharesDesc") }}</p><button class="btn btn-primary" @click="openWizard">{{ t("cloud.v4.newShare") }}</button></div>
        <div v-else class="share-group-list">
          <article v-for="group in shareGroups" :key="group.public_id" class="share-group-row" data-test="share-group-row" @click="openGroup(group)">
            <div class="group-main"><span class="group-icon"><Icon name="database" :size="18" /></span><div><div class="group-name"><strong>{{ group.name }}</strong><span class="badge" :class="group.status === 'active' ? 'badge-success' : 'badge-neutral'">{{ t(`cloud.v4.groupStatus.${group.status}`) }}</span></div><p>{{ group.description || t("cloud.v4.noDescription") }}</p></div></div>
            <div class="group-facts"><span><Icon name="accounts" :size="13" />{{ t("cloud.v4.accountCount", { count: group.account_count }) }}</span><span><Icon name="link" :size="13" />{{ t("cloud.v4.friendCount", { count: group.recipient_count }) }}</span><span><Icon name="activity" :size="13" />{{ group.default_rpm }} RPM · {{ group.default_concurrency }}</span></div>
            <div class="group-usage"><small>{{ t("cloud.v4.usedRequests") }}</small><strong>{{ group.used_requests || 0 }} / {{ group.default_quota_requests || "∞" }}</strong></div>
            <div class="group-actions" @click.stop><button class="btn btn-ghost btn-sm" :disabled="actionBusy !== ''" @click="toggleGroup(group)">{{ t(group.status === "active" ? "cloud.v4.pause" : "cloud.v4.resume") }}</button><button class="icon-button" :title="t('common.view')" @click="openGroup(group)"><Icon name="external" :size="15" /></button></div>
          </article>
        </div>
      </template>

      <template v-else-if="tab === 'received'">
        <section class="section-title page-section-title"><div><h2>{{ t("cloud.v4.receivedTitle") }}</h2><p>{{ t("cloud.v4.receivedDesc") }}</p></div></section>
        <div v-if="!receivedShares.length" class="feature-empty"><Icon name="download" :size="28" /><h3>{{ t("cloud.v4.noReceived") }}</h3><p>{{ t("cloud.v4.noReceivedDesc") }}</p></div>
        <div v-else class="received-list">
          <article v-for="share in receivedShares" :key="share.public_id" class="received-row" data-test="received-share-row" :class="`state-${share.status}`">
            <div class="received-heading"><div><span class="friend-avatar">{{ share.owner.display_name.slice(0, 1).toUpperCase() }}</span><div><h3>{{ share.group.name }}</h3><p>{{ t("cloud.v4.sharedBy", { name: share.owner.display_name }) }}</p></div></div><span class="badge" :class="share.status === 'active' ? 'badge-success' : share.status === 'pending' ? 'badge-warn' : 'badge-neutral'">{{ t(`cloud.v4.recipientStatus.${share.status}`) }}</span></div>
            <div class="received-policy"><span>{{ t("cloud.v4.accountCount", { count: share.group.account_count }) }}</span><span>{{ share.rpm_limit }} RPM</span><span>{{ t("cloud.v4.concurrent", { count: share.concurrency_limit }) }}</span><span>{{ share.used_requests }} / {{ share.quota_requests || "∞" }}</span></div>
            <div v-if="share.status === 'pending'" class="received-actions"><button class="btn btn-ghost" :disabled="actionBusy !== ''" @click="receivedAction(share, 'decline')">{{ t("cloud.v4.decline") }}</button><button class="btn btn-primary" data-test="accept-received-share" :disabled="actionBusy !== ''" @click="receivedAction(share, 'accept')"><Icon name="check" :size="14" />{{ t("cloud.v4.acceptShare") }}</button></div>
            <div v-else-if="share.status === 'active' || share.status === 'paused'" class="credential-area">
              <div><label>Base URL</label><code>{{ share.base_url }}</code><button class="icon-button" :title="t('common.copy')" @click="copy(share.base_url)"><Icon name="copy" :size="14" /></button></div>
              <div><label>API Key</label><code>{{ revealKeys[share.public_id] ? (share.api_key || "—") : `${share.key?.key_prefix || "sk-amber-"}••••••••` }}</code><button class="text-action" @click="revealKeys[share.public_id] = !revealKeys[share.public_id]">{{ t(revealKeys[share.public_id] ? "cloud.v4.hide" : "cloud.v4.show") }}</button><button class="icon-button" :title="t('common.copy')" :disabled="!share.api_key" @click="copy(share.api_key || '')"><Icon name="copy" :size="14" /></button></div>
              <div class="received-credential-actions"><button class="btn btn-ghost btn-sm" data-test="test-received-share" :disabled="actionBusy !== ''" @click="testReceivedShare(share)"><Icon name="activity" :size="13" />{{ actionBusy === `test-${share.public_id}` ? t("cloud.v4.testingShare") : t("cloud.v4.testShare") }}</button><button class="text-action leave-action" @click="receivedAction(share, 'leave')">{{ t("cloud.v4.leaveShare") }}</button></div>
            </div>
          </article>
        </div>
      </template>

      <template v-else-if="tab === 'friends'">
        <section class="section-title page-section-title"><div><h2>{{ t("cloud.v4.friendsTitle") }}</h2><p>{{ t("cloud.v4.friendsDesc") }}</p></div><button class="btn btn-primary" data-test="add-friend-open" @click="addFriendOpen = true"><Icon name="plus" :size="15" />{{ t("cloud.v4.addFriend") }}</button></section>
        <div class="friend-code-band"><div><span>{{ t("cloud.v4.yourFriendCode") }}</span><strong class="mono">{{ profile?.friend_code }}</strong></div><button class="btn btn-ghost btn-sm" :disabled="!profile?.friend_code" @click="copy(profile?.friend_code || '')"><Icon name="copy" :size="14" />{{ t("common.copy") }}</button></div>
        <section v-if="incomingRequests.length" class="workspace-section"><div class="section-title"><h3>{{ t("cloud.v4.incomingRequests") }} <span class="count-inline">{{ incomingRequests.length }}</span></h3></div><div class="friend-request-list"><article v-for="request in incomingRequests" :key="request.public_id"><span class="friend-avatar">{{ request.display_name.slice(0, 1).toUpperCase() }}</span><div><strong>{{ request.display_name }}</strong><small class="mono">{{ request.friend_code }}</small></div><div><button class="btn btn-ghost btn-sm" @click="friendRequestAction(request, 'decline')">{{ t("cloud.v4.decline") }}</button><button class="btn btn-primary btn-sm" @click="friendRequestAction(request, 'accept')">{{ t("cloud.v4.accept") }}</button></div></article></div></section>
        <section class="workspace-section"><div class="section-title"><h3>{{ t("cloud.v4.allFriends") }}</h3><span class="faint">{{ friends.length }}</span></div><div v-if="friends.length" class="friend-list"><article v-for="friend in friends" :key="friend.public_id"><span class="friend-avatar">{{ friend.display_name.slice(0, 1).toUpperCase() }}</span><div><strong>{{ friend.alias || friend.display_name }}</strong><small class="mono">{{ friend.friend_code }}</small></div><button class="btn btn-ghost btn-sm" @click="openWizard(); wizard.recipients = [{ friendship_id: friend.public_id }]">{{ t("cloud.v4.shareWithFriend") }}</button><button class="icon-button danger-text" :title="t('common.delete')" @click="removeFriend(friend)"><Icon name="trash" :size="14" /></button></article></div><div v-else class="feature-empty small"><Icon name="accounts" :size="24" /><p>{{ t("cloud.v4.noFriends") }}</p></div></section>
        <section v-if="outgoingRequests.length" class="workspace-section"><div class="section-title"><h3>{{ t("cloud.v4.outgoingRequests") }}</h3></div><div class="friend-request-list"><article v-for="request in outgoingRequests" :key="request.public_id"><span class="friend-avatar muted">{{ request.display_name.slice(0, 1).toUpperCase() }}</span><div><strong>{{ request.display_name }}</strong><small>{{ t("cloud.v4.waitingAccept") }}</small></div><button class="btn btn-ghost btn-sm" @click="friendRequestAction(request, 'cancel')">{{ t("common.cancel") }}</button></article></div></section>
      </template>

      <template v-else-if="tab === 'devices'">
        <section class="section-title page-section-title"><div><h2>{{ t("cloud.v4.devicesTitle") }}</h2><p>{{ t("cloud.v4.devicesDesc") }}</p></div><button v-if="!devices.length" class="btn btn-primary" :disabled="actionBusy !== ''" @click="ensureDevice"><Icon name="plus" :size="14" />{{ t("cloud.v4.registerDevice") }}</button></section>
        <div class="relay-control"><div><span class="relay-control-icon"><Icon name="server" :size="20" /></span><div><strong>{{ t("cloud.v4.ownerRelay") }}</strong><p>{{ t("cloud.v4.ownerRelayDesc") }}</p></div></div><button class="switch" :class="{ on: relayEnabled }" type="button" role="switch" :aria-checked="relayEnabled" :disabled="!devices.length || actionBusy !== ''" @click="toggleRelay"><span></span></button></div>
        <div v-if="devices.length" class="device-list"><article v-for="device in devices" :key="device.public_id"><span class="device-icon"><Icon name="server" :size="18" /></span><div><div><strong>{{ device.name }}</strong><span v-if="device.is_primary" class="badge badge-neutral">{{ t("cloud.v4.primaryDevice") }}</span></div><p>{{ device.online ? t("cloud.v4.deviceOnline") : t("cloud.v4.deviceOffline") }} · {{ device.capabilities.join(" / ") }}</p></div><span class="status-dot" :class="device.online ? 'active' : 'paused'"></span></article></div><div v-else class="feature-empty"><Icon name="server" :size="28" /><h3>{{ t("cloud.v4.noDevices") }}</h3><p>{{ t("cloud.v4.noDevicesDesc") }}</p></div>
      </template>

      <template v-else>
        <section class="section-title page-section-title"><div><h2>{{ t("cloud.securityTitle") }}</h2><p>{{ t("cloud.securityDesc") }}</p></div><button class="btn btn-ghost" @click="emit('password')"><Icon name="key" :size="14" />{{ t("cloud.changePassword") }}</button></section>
        <section class="security-list"><article><span><Icon name="key" :size="18" /></span><div><strong>{{ t("cloud.v4.identityKey") }}</strong><p>{{ t("cloud.v4.identityKeyDesc") }}</p></div><span class="badge badge-success">{{ t("cloud.v4.ready") }}</span></article><article><span><Icon name="refresh" :size="18" /></span><div><strong>{{ t("cloud.v4.syncState") }}</strong><p>{{ t("cloud.v4.syncStateDesc", { pending: status.pending_items, conflicts: status.conflicts.length }) }}</p></div><span class="badge" :class="status.last_error ? 'badge-danger' : 'badge-success'">{{ status.last_error ? t("cloud.syncFailed") : t("cloud.v4.ready") }}</span></article></section>
        <section v-if="status.conflicts.length" class="workspace-section"><div class="section-title"><h3>{{ t("cloud.conflictsTitle") }}</h3></div><div class="conflict-list-v4"><article v-for="conflict in status.conflicts" :key="conflict.id"><span class="badge badge-neutral">{{ t(`cloud.kind.${conflict.kind}`) }}</span><div><strong>{{ conflict.display_name || t(`cloud.kind.${conflict.kind}`) }}</strong><small>{{ fmt(conflict.created_at) }}</small></div></article></div></section>
      </template>
    </div>

    <div v-if="addFriendOpen" class="modal-backdrop" @click.self="addFriendOpen = false">
      <div class="modal compact-modal" role="dialog" aria-modal="true" @keydown.esc="addFriendOpen = false"><h3 class="modal-title">{{ t("cloud.v4.addFriend") }}</h3><p class="modal-desc">{{ t("cloud.v4.addFriendDesc") }}</p><label class="field"><span class="field-label">Friend Code</span><input v-model="friendCodeInput" class="input mono" data-test="friend-code-input" maxlength="13" placeholder="AMB-7K4P-N9Q2" @keyup.enter="sendFriendRequest" /></label><div class="modal-actions"><button class="btn btn-ghost" @click="addFriendOpen = false">{{ t("common.cancel") }}</button><button class="btn btn-primary" data-test="send-friend-request" :disabled="actionBusy !== '' || friendCodeInput.length < 13" @click="sendFriendRequest">{{ t("cloud.v4.sendRequest") }}</button></div></div>
    </div>

    <div v-if="wizardOpen" class="modal-backdrop" @click.self="closeWizard">
      <div class="modal share-wizard" role="dialog" aria-modal="true" @keydown.esc="closeWizard">
        <header class="wizard-header"><div><h3>{{ t("cloud.v4.wizard.title") }}</h3><p>{{ t(`cloud.v4.wizard.step${wizardStep}`) }}</p></div><button class="icon-button" :title="t('common.close')" @click="closeWizard"><Icon name="stop" :size="14" /></button></header>
        <ol class="wizard-steps"><li v-for="step in 5" :key="step" :class="{ active: wizardStep === step, done: wizardStep > step }"><span>{{ wizardStep > step ? "✓" : step }}</span><small>{{ t(`cloud.v4.wizard.short${step}`) }}</small></li></ol>
        <div class="wizard-body">
          <div v-if="wizardStep === 1" class="wizard-form"><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.groupName") }}</span><input v-model="wizard.name" class="input" data-test="share-group-name" maxlength="40" /></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.description") }}</span><textarea v-model="wizard.description" class="input textarea" maxlength="120"></textarea></label><div class="inline-note"><Icon name="key" :size="16" /><span>{{ t("cloud.v4.wizard.isolatedKeys") }}</span></div></div>
          <div v-else-if="wizardStep === 2"><label class="field selection-search"><span class="field-label">{{ t("cloud.v4.searchAccounts") }}</span><input v-model="wizardSearch" class="input" type="search" /></label><div class="selection-list"><article v-for="account in filteredWizardAccounts" :key="account.id" :class="{ selected: selectedAccount(account.id), unavailable: account.status !== 'active' }"><label><input type="checkbox" :data-test="`wizard-account-${account.id}`" :checked="selectedAccount(account.id)" :disabled="account.status !== 'active'" @change="toggleWizardAccount(account)" /><span class="account-type-icon"><Icon :name="account.account_type === 'oauth' ? 'cloud' : 'key'" :size="16" /></span><span><strong>{{ accountName(account) }}</strong><small>{{ t(`accounts.accountType.${account.account_type}`) }} · {{ t(`accounts.status.${account.status}`) }}</small></span></label><select v-if="selectedAccount(account.id) && account.account_type === 'api_key'" class="input compact-select" :value="wizard.accounts.find(item => item.account_id === account.id)?.relay_mode" @change="setWizardRelay(account.id, ($event.target as HTMLSelectElement).value as 'owner_device' | 'worker_direct')"><option value="owner_device">{{ t("cloud.v4.wizard.localRelay") }}</option><option value="worker_direct">{{ t("cloud.v4.wizard.workerDirect") }}</option></select><span v-else-if="selectedAccount(account.id)" class="badge badge-neutral">{{ t("cloud.v4.wizard.localRelay") }}</span></article></div></div>
          <div v-else-if="wizardStep === 3"><label class="field selection-search"><span class="field-label">{{ t("cloud.v4.searchFriends") }}</span><input v-model="wizardSearch" class="input" type="search" /></label><div class="selection-list"><article v-for="friend in filteredWizardFriends" :key="friend.public_id" :class="{ selected: selectedFriend(friend.public_id) }"><label><input type="checkbox" :data-test="`wizard-friend-${friend.public_id}`" :checked="selectedFriend(friend.public_id)" @change="toggleWizardFriend(friend)" /><span class="friend-avatar">{{ friend.display_name.slice(0, 1).toUpperCase() }}</span><span><strong>{{ friend.alias || friend.display_name }}</strong><small class="mono">{{ friend.friend_code }}</small></span></label></article><div v-if="!friends.length" class="feature-empty small"><p>{{ t("cloud.v4.wizard.noFriends") }}</p><button class="btn btn-ghost" @click="wizardOpen = false; tab = 'friends'; addFriendOpen = true">{{ t("cloud.v4.addFriend") }}</button></div></div></div>
          <div v-else-if="wizardStep === 4" class="rules-grid"><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.routePolicy") }}</span><select v-model="wizard.route_policy" class="input"><option value="balanced">{{ t("cloud.v4.wizard.balanced") }}</option><option value="failover">{{ t("cloud.v4.wizard.failover") }}</option></select></label><label class="field"><span class="field-label">RPM</span><input v-model.number="wizard.default_rpm" class="input" type="number" min="1" max="600" /></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.maxConcurrency") }}</span><input v-model.number="wizard.default_concurrency" class="input" type="number" min="1" max="20" /></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.totalQuota") }}</span><input v-model.number="wizard.default_quota_requests" class="input" type="number" min="0" max="1000000" /><small>{{ t("cloud.v4.wizard.zeroUnlimited") }}</small></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.expires") }}</span><input v-model="wizard.default_expires_at" class="input" type="date" /></label></div>
          <div v-else class="wizard-review"><section><span>{{ t("cloud.v4.wizard.groupName") }}</span><strong>{{ wizard.name }}</strong><small>{{ wizard.description || t("cloud.v4.noDescription") }}</small></section><section><span>{{ t("cloud.v4.wizard.accounts") }}</span><strong>{{ wizard.accounts.length }}</strong><small>{{ wizard.accounts.map(item => accountName(accounts.find(account => account.id === item.account_id)!)).join(" · ") }}</small></section><section><span>{{ t("cloud.v4.wizard.friends") }}</span><strong>{{ wizard.recipients.length }}</strong><small>{{ wizard.recipients.map(item => friends.find(friend => friend.public_id === item.friendship_id)?.display_name).join(" · ") }}</small></section><section><span>{{ t("cloud.v4.wizard.rules") }}</span><strong>{{ wizard.default_rpm }} RPM · {{ wizard.default_concurrency }}</strong><small>{{ t("cloud.v4.wizard.reviewQuota", { quota: wizard.default_quota_requests || "∞" }) }}</small></section><div v-if="wizard.accounts.some(item => item.relay_mode === 'owner_device')" class="inline-note warn"><Icon name="warn" :size="16" /><span>{{ t("cloud.v4.wizard.ownerOnlineWarning") }}</span></div></div>
        </div>
        <footer class="wizard-footer"><button class="btn btn-ghost" :disabled="wizardStep === 1 || actionBusy !== ''" @click="wizardStep--; wizardSearch = ''">{{ t("common.back") }}</button><span>{{ wizardStep }} / 5</span><button v-if="wizardStep < 5" class="btn btn-primary" data-test="wizard-next" @click="nextWizardStep(); wizardSearch = ''">{{ t("common.next") }}</button><button v-else class="btn btn-primary" data-test="wizard-create" :disabled="actionBusy !== ''" @click="createShareGroup"><Icon name="check" :size="14" />{{ actionBusy === "create-group" ? t("cloud.v4.wizard.creating") : t("cloud.v4.wizard.create") }}</button></footer>
      </div>
    </div>

    <div v-if="accountPickerOpen" class="modal-backdrop" @click.self="accountPickerOpen = false">
      <div class="modal compact-modal" role="dialog" aria-modal="true">
        <h3 class="modal-title">{{ t("cloud.v4.addAccount") }}</h3>
        <p class="modal-desc">{{ t("cloud.v4.addAccountDesc") }}</p>
        <label class="field"><span class="field-label">{{ t("cloud.v4.wizard.accounts") }}</span><select v-model.number="accountPickerID" class="input"><option v-for="account in availableDetailAccounts" :key="account.id" :value="account.id">{{ accountName(account) }}</option></select></label>
        <label class="field"><span class="field-label">{{ t("cloud.v4.relayRoute") }}</span><select v-model="accountPickerRelay" class="input" :disabled="selectedPickerAccount()?.account_type === 'oauth'"><option value="owner_device">{{ t("cloud.v4.wizard.localRelay") }}</option><option value="worker_direct">{{ t("cloud.v4.wizard.workerDirect") }}</option></select></label>
        <p v-if="selectedPickerAccount()?.account_type === 'oauth'" class="modal-desc">{{ t("cloud.v4.oauthRelayRequired") }}</p>
        <div class="modal-actions"><button class="btn btn-ghost" @click="accountPickerOpen = false">{{ t("common.cancel") }}</button><button class="btn btn-primary" :disabled="!accountPickerID || actionBusy !== ''" @click="addGroupAccount">{{ t("cloud.v4.addAccount") }}</button></div>
      </div>
    </div>

    <div v-if="invitePickerOpen" class="modal-backdrop" @click.self="invitePickerOpen = false">
      <div class="modal compact-modal" role="dialog" aria-modal="true">
        <h3 class="modal-title">{{ t("cloud.v4.inviteFriends") }}</h3>
        <p class="modal-desc">{{ t("cloud.v4.inviteFriendsDesc") }}</p>
        <div class="selection-list modal-selection"><article v-for="friend in availableDetailFriends" :key="friend.public_id" :class="{ selected: inviteFriendIDs.includes(friend.public_id) }"><label><input type="checkbox" :checked="inviteFriendIDs.includes(friend.public_id)" @change="toggleInviteFriend(friend.public_id)" /><span class="friend-avatar">{{ friend.display_name.slice(0, 1).toUpperCase() }}</span><span><strong>{{ friend.alias || friend.display_name }}</strong><small class="mono">{{ friend.friend_code }}</small></span></label></article></div>
        <div class="modal-actions"><button class="btn btn-ghost" @click="invitePickerOpen = false">{{ t("common.cancel") }}</button><button class="btn btn-primary" :disabled="!inviteFriendIDs.length || actionBusy !== ''" @click="inviteGroupFriends">{{ t("cloud.v4.sendInvite") }}</button></div>
      </div>
    </div>

    <div v-if="recipientEditor" class="modal-backdrop" @click.self="recipientEditor = null">
      <div class="modal compact-modal" role="dialog" aria-modal="true">
        <h3 class="modal-title">{{ t("cloud.v4.editRulesFor", { name: recipientEditor.display_name }) }}</h3>
        <p class="modal-desc">{{ t("cloud.v4.editRulesDesc") }}</p>
        <div class="rules-grid"><label class="field"><span class="field-label">RPM</span><input v-model.number="recipientRules.rpm_limit" class="input" type="number" min="1" max="600" /></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.maxConcurrency") }}</span><input v-model.number="recipientRules.concurrency_limit" class="input" type="number" min="1" max="20" /></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.totalQuota") }}</span><input v-model.number="recipientRules.quota_requests" class="input" type="number" min="0" max="1000000" /><small>{{ t("cloud.v4.wizard.zeroUnlimited") }}</small></label><label class="field"><span class="field-label">{{ t("cloud.v4.wizard.expires") }}</span><input v-model="recipientRules.expires_at" class="input" type="date" /></label></div>
        <div class="modal-actions"><button class="btn btn-ghost" @click="recipientEditor = null">{{ t("common.cancel") }}</button><button class="btn btn-primary" :disabled="actionBusy !== ''" @click="saveRecipientRules">{{ t("common.save") }}</button></div>
      </div>
    </div>

    <div v-if="detailOpen && detail" class="drawer-backdrop" @click.self="detailOpen = false">
      <aside class="detail-drawer" data-test="share-group-detail" role="dialog" aria-modal="true">
        <header><div><div><h2>{{ detail.group.name }}</h2><span class="badge" :class="detail.group.status === 'active' ? 'badge-success' : 'badge-neutral'">{{ t(`cloud.v4.groupStatus.${detail.group.status}`) }}</span></div><p>{{ detail.group.description || t("cloud.v4.noDescription") }}</p></div><button class="icon-button" :title="t('common.close')" @click="detailOpen = false"><Icon name="stop" :size="14" /></button></header>
        <div class="drawer-toolbar"><button class="btn btn-ghost btn-sm" @click="toggleGroup(detail.group)">{{ t(detail.group.status === "active" ? "cloud.v4.pause" : "cloud.v4.resume") }}</button><button v-if="availableDetailAccounts.length" class="btn btn-ghost btn-sm" @click="openAccountPicker"><Icon name="plus" :size="13" />{{ t("cloud.v4.addAccount") }}</button><button v-if="availableDetailFriends.length" class="btn btn-ghost btn-sm" @click="openInvitePicker"><Icon name="accounts" :size="13" />{{ t("cloud.v4.inviteFriends") }}</button><button class="btn btn-danger btn-sm" @click="deleteGroupTarget = detail.group"><Icon name="trash" :size="13" />{{ t("common.delete") }}</button></div>
        <nav><button v-for="item in (['accounts','friends','usage'] as const)" :key="item" :class="{ active: detailTab === item }" @click="detailTab = item">{{ t(`cloud.v4.detail.${item}`) }}</button></nav>
        <div class="drawer-body">
          <div v-if="detailTab === 'accounts'" class="drawer-list"><article v-for="account in detail.accounts" :key="account.public_id"><span class="account-type-icon"><Icon :name="account.account_type === 'oauth' ? 'cloud' : 'key'" :size="16" /></span><div><strong class="mono">{{ account.account_uid }}</strong><small>{{ t(`accounts.accountType.${account.account_type}`) }} · {{ t(`cloud.v4.relayMode.${account.relay_mode}`) }}</small></div><button class="btn btn-ghost btn-sm" @click="toggleGroupAccount(account.public_id, account.enabled)">{{ t(Boolean(account.enabled) ? "cloud.v4.disable" : "cloud.v4.enable") }}</button><button class="icon-button danger-text" :title="t('common.delete')" @click="removeGroupAccount(account.public_id)"><Icon name="trash" :size="14" /></button></article></div>
          <div v-else-if="detailTab === 'friends'" class="drawer-list"><article v-for="recipient in detail.recipients" :key="recipient.public_id"><span class="friend-avatar">{{ recipient.display_name.slice(0, 1).toUpperCase() }}</span><div><strong>{{ recipient.display_name }}</strong><small>{{ recipient.used_requests }} / {{ recipient.quota_requests || "∞" }} · {{ recipient.rpm_limit }} RPM · {{ recipient.concurrency_limit }} · {{ recipient.key_prefix }}</small></div><span class="badge" :class="recipient.status === 'active' ? 'badge-success' : 'badge-neutral'">{{ t(`cloud.v4.recipientStatus.${recipient.status}`) }}</span><button v-if="recipient.status === 'active' || recipient.status === 'paused'" class="icon-button" :title="t('cloud.v4.editRules')" @click="editRecipient(recipient)"><Icon name="settings" :size="14" /></button><button v-if="recipient.status === 'active' || recipient.status === 'paused'" class="btn btn-ghost btn-sm" @click="toggleRecipient(recipient)">{{ t(recipient.status === "active" ? "cloud.v4.pause" : "cloud.v4.resume") }}</button><button v-if="recipient.status === 'active' || recipient.status === 'paused'" class="icon-button" :title="t('cloud.v4.rotateKey')" @click="rotateRecipient(recipient)"><Icon name="refresh" :size="14" /></button><button class="icon-button danger-text" :title="t('cloud.v4.revoke')" @click="revokeRecipient(recipient)"><Icon name="trash" :size="14" /></button></article></div>
          <div v-else><div class="usage-summary"><div><span>{{ t("cloud.v4.usedRequests") }}</span><strong>{{ detail.recipients.reduce((sum, item) => sum + item.used_requests, 0) }}</strong></div><div><span>RPM</span><strong>{{ detail.group.default_rpm }}</strong></div><div><span>{{ t("cloud.v4.concurrent", { count: "" }) }}</span><strong>{{ detail.group.default_concurrency }}</strong></div></div><div v-if="detailUsageLoading" class="plain-empty compact">{{ t("common.loading") }}</div><div v-else-if="!detailUsage.length" class="plain-empty compact">{{ t("cloud.v4.noUsage") }}</div><div v-else class="usage-event-list"><article v-for="entry in detailUsage" :key="entry.request_id"><span class="status-dot" :class="entry.status < 400 ? 'active' : 'paused'"></span><div><strong>{{ entry.model || "—" }}</strong><small>{{ entry.display_name }} · {{ entry.route_mode }} · {{ fmt(entry.created_at) }}</small></div><code>{{ entry.status }} · {{ entry.latency_ms }}ms</code></article></div></div>
        </div>
      </aside>
    </div>

    <ConfirmModal :open="Boolean(deleteGroupTarget)" :title="t('cloud.v4.deleteShareTitle')" :desc="t('cloud.v4.deleteShareDesc', { name: deleteGroupTarget?.name || '' })" :confirm-text="t('common.delete')" danger @confirm="deleteGroup" @cancel="deleteGroupTarget = null" />
  </section>
</template>

<style scoped>
.cloud-workspace { width: 100%; max-width: 1240px; margin: 0 auto; }
.workspace-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding-bottom: 18px; }
.workspace-identity, .workspace-actions, .received-heading > div, .relay-control > div { display: flex; align-items: center; gap: 11px; }
.workspace-avatar { width: 42px; height: 42px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 8px; background: var(--primary-soft); color: var(--primary); font-size: 17px; font-weight: 750; }
.workspace-identity h1 { margin: 0; font-size: 18px; }
.workspace-identity p, .section-title p, .received-heading p, .relay-control p { margin: 3px 0 0; color: var(--text-dim); }
.workspace-actions { justify-content: flex-end; flex-wrap: wrap; }
.icon-only { width: 36px; padding: 0; }
.relay-state { display: inline-flex; align-items: center; gap: 6px; color: var(--text-dim); font-size: 12px; }
.relay-state i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
.relay-state.online i { background: var(--success); box-shadow: 0 0 0 3px var(--success-soft); }
.workspace-tabs { display: flex; gap: 2px; overflow-x: auto; border-bottom: 1px solid var(--border); }
.workspace-tabs button { position: relative; min-width: max-content; min-height: 42px; display: inline-flex; align-items: center; gap: 7px; padding: 0 14px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--text-dim); font-weight: 600; cursor: pointer; }
.workspace-tabs button.active { border-bottom-color: var(--primary); color: var(--text); }
.tab-count, .count-inline { min-width: 19px; height: 19px; display: inline-grid; place-items: center; padding: 0 5px; border-radius: 10px; background: var(--danger-soft); color: var(--danger); font-size: 10px; }
.workspace-content { min-width: 0; padding-top: 22px; }
.workspace-loading { display: grid; gap: 10px; padding-top: 20px; }
.workspace-loading span { height: 76px; border-radius: 6px; background: var(--bg-elev); }
.workspace-error { min-height: 140px; display: flex; align-items: center; justify-content: center; gap: 10px; color: var(--danger); }
.sync-error-band { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 12px 4px; border-bottom: 1px solid var(--danger); color: var(--danger); }.sync-error-band p { margin: 3px 0 5px; color: var(--text-dim); }.sync-error-band span { display: inline-block; margin-right: 14px; color: var(--text-faint); font-size: 11px; }
.sync-error-actions { display: flex; align-items: center; gap: 8px; }
.metric-strip { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); border-block: 1px solid var(--border-soft); }
.metric-strip div { min-width: 0; display: grid; gap: 5px; padding: 16px 18px; border-right: 1px solid var(--border-soft); }
.metric-strip div:last-child { border-right: 0; }
.metric-strip span { color: var(--text-faint); font-size: 11px; }
.metric-strip strong { font-size: 21px; }
.metric-strip .metric-time { overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.workspace-section { margin-top: 28px; }
.section-title { min-width: 0; display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.section-title h2, .section-title h3 { margin: 0; font-size: 15px; }
.page-section-title { margin-bottom: 16px; }
.text-action { padding: 4px; border: 0; background: transparent; color: var(--primary); font-size: 12px; cursor: pointer; }
.plain-empty { min-height: 90px; display: flex; align-items: center; justify-content: center; gap: 9px; margin-top: 10px; border-block: 1px solid var(--border-soft); color: var(--text-faint); }
.plain-empty.compact { min-height: 100px; margin-top: 8px; }
.task-list { margin-top: 10px; border-top: 1px solid var(--border-soft); }
.task-list button { width: 100%; min-height: 54px; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 11px; padding: 9px 4px; border: 0; border-bottom: 1px solid var(--border-soft); background: transparent; color: var(--text); text-align: left; cursor: pointer; }
.task-list button > span:nth-child(2) { display: grid; gap: 3px; color: var(--text-dim); font-size: 12px; }
.task-list strong { color: var(--text); }
.task-icon, .group-icon, .account-type-icon, .device-icon, .relay-control-icon { width: 34px; height: 34px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 7px; background: var(--primary-soft); color: var(--primary); }
.task-icon.danger { background: var(--danger-soft); color: var(--danger); }
.overview-columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 36px; }
.compact-list { margin-top: 8px; border-top: 1px solid var(--border-soft); }
.compact-list button { width: 100%; min-height: 54px; display: flex; align-items: center; gap: 10px; padding: 8px 3px; border: 0; border-bottom: 1px solid var(--border-soft); background: transparent; color: var(--text); text-align: left; cursor: pointer; }
.compact-list button > span:nth-child(2) { min-width: 0; display: grid; gap: 3px; flex: 1; }
.compact-list small { overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }
.status-dot { width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%; background: var(--text-faint); }
.status-dot.active { background: var(--success); }.status-dot.paused { background: var(--warn); }
.friend-avatar { width: 34px; height: 34px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 50%; background: var(--bg-elev); color: var(--text-dim); font-weight: 700; }
.friend-avatar.muted { opacity: .65; }
.feature-empty { min-height: 260px; display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 8px; border-block: 1px solid var(--border-soft); color: var(--text-faint); text-align: center; }
.feature-empty h3 { margin: 4px 0 0; color: var(--text); font-size: 15px; }.feature-empty p { max-width: 430px; margin: 0; }.feature-empty.small { min-height: 140px; }
.share-group-list, .received-list, .device-list { border-top: 1px solid var(--border-soft); }
.share-group-row { display: grid; grid-template-columns: minmax(240px, 1.4fr) minmax(250px, 1fr) minmax(120px, auto) auto; align-items: center; gap: 18px; padding: 16px 4px; border-bottom: 1px solid var(--border-soft); cursor: pointer; transition: border-color 150ms ease, transform 150ms ease, box-shadow 150ms ease; }
.share-group-row:hover { transform: translateY(-1px); border-color: var(--border); box-shadow: 0 5px 14px rgba(20, 22, 28, .05); }
.group-main { min-width: 0; display: flex; align-items: center; gap: 11px; }.group-main > div { min-width: 0; }
.group-name { display: flex; align-items: center; gap: 8px; }.group-name strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-main p { overflow: hidden; margin: 4px 0 0; color: var(--text-faint); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.group-facts { display: flex; gap: 7px 14px; flex-wrap: wrap; color: var(--text-dim); font-size: 12px; }.group-facts span { display: inline-flex; align-items: center; gap: 4px; }
.group-usage { display: grid; gap: 3px; }.group-usage small { color: var(--text-faint); }.group-actions { display: flex; gap: 5px; }
.icon-button { width: 32px; height: 32px; display: inline-grid; place-items: center; padding: 0; border: 1px solid transparent; border-radius: 6px; background: transparent; color: var(--text-dim); cursor: pointer; }.icon-button:hover { border-color: var(--border); background: var(--bg-elev); color: var(--text); }.icon-button:disabled { opacity: .45; cursor: not-allowed; }
.received-row { padding: 17px 4px; border-bottom: 1px solid var(--border-soft); }.received-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }.received-heading h3 { margin: 0; font-size: 15px; }
.received-policy { display: flex; gap: 8px 18px; flex-wrap: wrap; margin: 12px 0 0 45px; color: var(--text-dim); font-size: 12px; }
.received-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.credential-area { display: grid; gap: 8px; margin: 14px 0 0 45px; }.credential-area > div { display: grid; grid-template-columns: 82px minmax(0, 1fr) auto auto; align-items: center; gap: 7px; padding: 7px 9px; border: 1px solid var(--border-soft); border-radius: 6px; background: var(--bg-elev); }.credential-area label { color: var(--text-faint); font-size: 11px; }.credential-area code { overflow: hidden; color: var(--text); text-overflow: ellipsis; white-space: nowrap; }.leave-action { justify-self: end; color: var(--danger); }
.credential-area > .received-credential-actions { display: flex; justify-content: flex-end; padding: 0; border: 0; background: transparent; }
.friend-code-band { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border-block: 1px solid var(--border-soft); background: var(--bg-elev); }.friend-code-band > div { display: grid; gap: 4px; }.friend-code-band span { color: var(--text-faint); font-size: 11px; }.friend-code-band strong { font-size: 16px; }
.friend-request-list, .friend-list { margin-top: 10px; border-top: 1px solid var(--border-soft); }.friend-request-list article, .friend-list article, .device-list article, .drawer-list article, .security-list article { min-height: 58px; display: flex; align-items: center; gap: 11px; padding: 9px 3px; border-bottom: 1px solid var(--border-soft); }.friend-request-list article > div:nth-child(2), .friend-list article > div { min-width: 0; display: grid; gap: 3px; flex: 1; }.friend-request-list small, .friend-list small, .device-list p, .drawer-list small { color: var(--text-faint); }.friend-request-list article > div:last-child { display: flex; gap: 6px; }
.relay-control { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 16px; border-block: 1px solid var(--border-soft); background: var(--bg-elev); }.relay-control p { max-width: 650px; }.switch { width: 42px; height: 24px; padding: 2px; border: 0; border-radius: 12px; background: var(--border); cursor: pointer; transition: background 150ms ease; }.switch span { width: 20px; height: 20px; display: block; border-radius: 50%; background: white; transition: transform 150ms ease; }.switch.on { background: var(--success); }.switch.on span { transform: translateX(18px); }.switch:disabled { opacity: .45; cursor: not-allowed; }
.device-list { margin-top: 18px; }.device-list article > div { min-width: 0; flex: 1; }.device-list article > div > div { display: flex; align-items: center; gap: 7px; }.device-list p { margin: 4px 0 0; }
.security-list { border-top: 1px solid var(--border-soft); }.security-list article > span:first-child { width: 34px; height: 34px; display: grid; place-items: center; color: var(--primary); }.security-list article > div { min-width: 0; flex: 1; }.security-list p { margin: 3px 0 0; color: var(--text-dim); }.conflict-list-v4 { border-top: 1px solid var(--border-soft); }.conflict-list-v4 article { display: flex; align-items: center; gap: 10px; padding: 10px 3px; border-bottom: 1px solid var(--border-soft); }.conflict-list-v4 article div { display: grid; gap: 2px; }
.compact-modal { max-width: 460px; }.textarea { min-height: 84px; resize: vertical; }
.share-wizard { width: min(760px, calc(100vw - 32px)); max-width: 760px; height: min(760px, calc(100vh - 36px)); display: grid; grid-template-rows: auto auto minmax(0, 1fr) auto; padding: 0; overflow: hidden; }.wizard-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 18px 20px 12px; }.wizard-header h3 { margin: 0; font-size: 17px; }.wizard-header p { margin: 4px 0 0; color: var(--text-dim); }.wizard-steps { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); margin: 0; padding: 0 20px 14px; list-style: none; }.wizard-steps li { position: relative; display: grid; justify-items: center; gap: 5px; color: var(--text-faint); font-size: 10px; }.wizard-steps li::before { content: ""; position: absolute; z-index: 0; top: 11px; right: 50%; width: 100%; height: 1px; background: var(--border); }.wizard-steps li:first-child::before { display: none; }.wizard-steps span { z-index: 1; width: 22px; height: 22px; display: grid; place-items: center; border: 1px solid var(--border); border-radius: 50%; background: var(--bg-card); }.wizard-steps li.active, .wizard-steps li.done { color: var(--primary); }.wizard-steps li.active span, .wizard-steps li.done span { border-color: var(--primary); background: var(--primary-soft); }.wizard-body { overflow-y: auto; padding: 20px; border-block: 1px solid var(--border-soft); }.wizard-form { display: grid; gap: 15px; }.inline-note { display: flex; align-items: flex-start; gap: 9px; padding: 11px 12px; border: 1px solid var(--border-soft); border-radius: 6px; background: var(--bg-elev); color: var(--text-dim); }.inline-note.warn { border-color: rgba(193,134,58,.3); background: var(--warn-soft); color: var(--warn); }
.selection-list { border-top: 1px solid var(--border-soft); }.selection-list article { min-height: 58px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 9px 5px; border-bottom: 1px solid var(--border-soft); }.selection-list article.selected { background: var(--primary-soft); }.selection-list article.unavailable { opacity: .55; }.selection-list label { min-width: 0; display: flex; align-items: center; gap: 10px; flex: 1; cursor: pointer; }.selection-list label > span:last-child { min-width: 0; display: grid; gap: 3px; }.selection-list small { overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }.compact-select { width: 150px; padding-block: 6px; }.rules-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }.rules-grid .field:last-child { grid-column: 1 / -1; }.wizard-review { display: grid; gap: 2px; }.wizard-review section { display: grid; grid-template-columns: 130px minmax(0, 1fr); gap: 4px 14px; padding: 12px 2px; border-bottom: 1px solid var(--border-soft); }.wizard-review section > span { grid-row: 1 / 3; color: var(--text-faint); font-size: 12px; }.wizard-review small { overflow-wrap: anywhere; color: var(--text-dim); }.wizard-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 13px 20px; }.wizard-footer > span { color: var(--text-faint); font-size: 11px; }
.selection-search { margin-bottom: 12px; }.modal-selection { max-height: 300px; overflow-y: auto; }
.drawer-backdrop { position: fixed; z-index: 1200; inset: 0; display: flex; justify-content: flex-end; background: rgba(12, 15, 20, .38); }.detail-drawer { width: min(720px, 94vw); height: 100%; display: grid; grid-template-rows: auto auto auto minmax(0, 1fr); background: var(--bg-card); box-shadow: -14px 0 36px rgba(8, 10, 14, .16); }.detail-drawer > header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 20px; border-bottom: 1px solid var(--border-soft); }.detail-drawer > header > div > div { display: flex; align-items: center; gap: 8px; }.detail-drawer h2 { margin: 0; font-size: 18px; }.detail-drawer header p { margin: 5px 0 0; color: var(--text-dim); }.drawer-toolbar { display: flex; gap: 7px; padding: 10px 20px; border-bottom: 1px solid var(--border-soft); }.detail-drawer > nav { display: flex; gap: 2px; padding: 0 20px; border-bottom: 1px solid var(--border-soft); }.detail-drawer > nav button { padding: 11px 12px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--text-dim); cursor: pointer; }.detail-drawer > nav button.active { border-bottom-color: var(--primary); color: var(--text); }.drawer-body { overflow-y: auto; padding: 8px 20px 28px; }.drawer-list article > div { min-width: 0; display: grid; gap: 3px; flex: 1; }.drawer-list .mono { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.usage-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); margin-top: 12px; border-block: 1px solid var(--border-soft); }.usage-summary div { display: grid; gap: 5px; padding: 18px; border-right: 1px solid var(--border-soft); }.usage-summary div:last-child { border-right: 0; }.usage-summary span { color: var(--text-faint); font-size: 11px; }.usage-summary strong { font-size: 20px; }
.drawer-toolbar { flex-wrap: wrap; }.usage-event-list { margin-top: 12px; border-top: 1px solid var(--border-soft); }.usage-event-list article { min-height: 54px; display: flex; align-items: center; gap: 10px; padding: 8px 3px; border-bottom: 1px solid var(--border-soft); }.usage-event-list article > div { min-width: 0; display: grid; gap: 3px; flex: 1; }.usage-event-list small { overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }.usage-event-list code { color: var(--text-dim); }
.danger-text { color: var(--danger); }.faint { color: var(--text-faint); }.mono { font-family: var(--font-mono); }
@media (prefers-reduced-motion: reduce) { .share-group-row, .switch, .switch span { transition: none; } }
@media (max-width: 980px) { .share-group-row { grid-template-columns: minmax(220px, 1fr) minmax(220px, 1fr) auto; }.group-usage { display: none; }.metric-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }.metric-strip div:nth-child(2) { border-right: 0; }.metric-strip div:nth-child(-n+2) { border-bottom: 1px solid var(--border-soft); } }
@media (max-width: 720px) { .workspace-header { align-items: flex-start; flex-direction: column; }.workspace-actions { width: 100%; justify-content: flex-start; }.relay-state { width: 100%; }.sync-error-band { grid-template-columns: auto minmax(0, 1fr); }.sync-error-actions { grid-column: 2; flex-wrap: wrap; justify-self: start; }.overview-columns, .rules-grid { grid-template-columns: minmax(0, 1fr); }.share-group-row { grid-template-columns: minmax(0, 1fr) auto; gap: 10px; }.group-facts { grid-column: 1 / -1; }.credential-area { margin-left: 0; }.credential-area > div { grid-template-columns: minmax(0, 1fr) auto auto; }.credential-area label { grid-column: 1 / -1; }.received-policy { margin-left: 0; }.wizard-steps small { display: none; }.share-wizard { width: 100%; height: 100%; max-height: none; border-radius: 0; }.selection-list article { align-items: stretch; flex-direction: column; }.selection-list label { width: 100%; }.compact-select { width: 100%; }.wizard-review section { grid-template-columns: minmax(0, 1fr); }.wizard-review section > span { grid-row: auto; }.detail-drawer { width: 100%; }.drawer-list article { align-items: flex-start; flex-wrap: wrap; }.usage-summary { grid-template-columns: minmax(0, 1fr); }.usage-summary div { border-right: 0; border-bottom: 1px solid var(--border-soft); } }
</style>
