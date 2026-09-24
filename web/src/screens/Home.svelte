<script lang="ts">
  import { api, type PublicConfig } from '../lib/api'
  import Calendar from './Calendar.svelte'
  import Login from './Login.svelte'

  // The front page is the public calendar while it is switched on.
  // Otherwise (404 or public_calendar false) it is the login.
  let config = $state<PublicConfig | null>(null)
  let off = $state(false)

  api
    .publicConfig()
    .then((c) => {
      if (c.public_calendar) config = c
      else off = true
    })
    .catch(() => (off = true))
</script>

{#if config}
  <Calendar {config} />
{:else if off}
  <Login calendarOff />
{/if}
