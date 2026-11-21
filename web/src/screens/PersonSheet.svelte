<script lang="ts">
  // A person in a narrow window: the same detail as the pane beside the
  // list, sliding in over the list, closed with its cross or Escape. The
  // name stands in the bar beside the cross, as it does in the pane.
  import type { PersonDetail as Detail } from '../lib/api'
  import Sheet from '../lib/components/Sheet.svelte'
  import PersonDetail from './PersonDetail.svelte'

  let { id, name = '', onclose, onupdate }: { id: number; name?: string; onclose: () => void; onupdate?: (p: Detail) => void } = $props()
  // The parent keys this by id, so the name from the list holds until
  // the person's own arrives.
  // svelte-ignore state_referenced_locally
  let shown = $state(name)
</script>

<Sheet label={shown || 'Person'} personal {onclose}>
  {#snippet head()}
    <h2 class="person-name ellipsis" data-personal title={shown}>{shown}</h2>
  {/snippet}
  <PersonDetail {id} {onupdate} onname={(n) => (shown = n)} />
</Sheet>
