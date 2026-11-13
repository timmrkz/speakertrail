<script lang="ts">
  // A person in a narrow window: the same detail as the pane beside the
  // list, sliding in over the list, closed with its cross or Escape.
  import type { PersonDetail as Detail } from '../lib/api'
  import Sheet from '../lib/components/Sheet.svelte'
  import PersonDetail from './PersonDetail.svelte'

  let { id, name = 'Person', onclose, onupdate }: { id: number; name?: string; onclose: () => void; onupdate?: (p: Detail) => void } = $props()
</script>

<Sheet label={name} {onclose}>
  <PersonDetail {id} {onupdate} />
</Sheet>
