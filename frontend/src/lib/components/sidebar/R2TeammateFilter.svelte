<script lang="ts">
  import { onMount } from "svelte";
  import { Button, Typeahead } from "@kenn-io/kit-ui";
  import { SessionsService, type R2User } from "../../api/generated/index.js";
  import { m } from "../../i18n/index.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { failureMessage } from "../../utils/failure.js";
  import { teammateLabel } from "../../utils/teammates.js";
  import LoadError from "../shared/LoadError.svelte";

  let users: R2User[] = $state([]);
  let configured = $state(false);
  let loading = $state(false);
  let error: string | null = $state(null);
  const value = $derived(sessions.filters.r2UserID ?? "");
  const options = $derived.by(() => {
    const known = new Map(users.map((user) => [user.user_id, teammateLabel(user)]));
    for (const row of sessions.sessions) {
      const user = row.r2_teammate;
      if (user && !known.has(user.user_id)) known.set(user.user_id, teammateLabel(user));
    }
    if (value && !known.has(value)) known.set(value, value);
    const entries = [...known].map(([name, label]) => ({ name, label, displayLabel: label }));
    entries.sort((a, b) => a.label.localeCompare(b.label));
    const all = m.sidebar_r2_all_users();
    return [{ name: "", label: all, displayLabel: all }, ...entries];
  });

  async function load(refresh: boolean, signal?: AbortSignal) {
    loading = true;
    try {
      const result = refresh
        ? await SessionsService.postApiV1R2TeammatesRefresh({ signal })
        : await SessionsService.getApiV1R2Teammates({ signal });
      configured = result.configured;
      users = result.users;
      error = result.error ?? null;
      if (refresh) {
        void sessions.load({ force: true });
        void sessions.refreshActiveSession();
      }
    } catch (cause) {
      if (!signal?.aborted && (refresh || options.length > 1)) error = failureMessage(cause);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    const controller = new AbortController();
    void load(false, controller.signal);
    return () => controller.abort();
  });
</script>

{#if configured || options.length > 1}
  <div class="teammate-filter">
    <Typeahead
      {options}
      {value}
      title={m.sidebar_r2_teammate()}
      triggerPrefix={m.sidebar_r2_teammate()}
      fallbackLabel={value || m.sidebar_r2_all_users()}
      placeholder={m.sidebar_r2_search_users()}
      emptyLabel={m.sidebar_r2_no_users()}
      onselect={(id) => sessions.setR2UserFilter(id)}
    />
    {#if configured}
      <Button size="sm" disabled={loading} onclick={() => void load(true)}>
        {m.sidebar_r2_refresh_users()}
      </Button>
    {/if}
  </div>
  {#if error}
    <LoadError message={error} onRetry={() => void load(true)} />
  {/if}
{/if}

<style>
  .teammate-filter {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2, 8px);
    padding: var(--space-2, 8px);
  }
</style>
