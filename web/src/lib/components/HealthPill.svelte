<script lang="ts">
  import type { Health } from '../api'

  let { health }: { health: Health } = $props()
  const TONE: Record<Health, string> = { ok: 'good', warning: 'warn', error: 'bad', never: '' }
  const LABEL: Record<Health, string> = { ok: 'OK', warning: 'Warning', error: 'Error', never: 'Not checked yet' }
</script>

<span class="pill {TONE[health]}"><span class="dot"></span>{LABEL[health]}</span>
