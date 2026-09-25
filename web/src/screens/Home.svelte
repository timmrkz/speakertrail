<script lang="ts">
  import { api, type PublicConfig } from '../lib/api'
  import Calendar from './Calendar.svelte'

  // The front page is always the calendar. While it is closed, visitors
  // see that it opens soon and Tim, logged in, sees it already.
  const CLOSED: PublicConfig = { public_calendar: false, owner: false, show_people: false, cities: [] }
  let config = $state<PublicConfig | null>(null)

  api
    .publicConfig()
    .then((c) => (config = c))
    .catch(() => (config = CLOSED))
</script>

{#if config}
  <Calendar {config} />
{/if}
