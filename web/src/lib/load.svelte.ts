// Loads data for a screen and keeps only the newest answer when several
// requests overlap, for example while typing in a search field.

export class Load<T> {
  data = $state<T | undefined>(undefined)
  loading = $state(false)
  error = $state('')
  private seq = 0

  async run(fetcher: () => Promise<T>): Promise<void> {
    const mine = ++this.seq
    this.loading = true
    this.error = ''
    try {
      const data = await fetcher()
      if (mine === this.seq) this.data = data
    } catch (e) {
      if (mine === this.seq) this.error = e instanceof Error ? e.message : 'Something went wrong'
    } finally {
      if (mine === this.seq) this.loading = false
    }
  }
}
