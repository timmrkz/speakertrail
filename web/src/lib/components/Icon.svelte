<script lang="ts" module>
  // Every glyph in the app is a Lucide icon, the set FrameFairy draws
  // with, so the two apps share one hand and none of the icons is one we
  // made up. The app knows them by what they are for, here, and nowhere
  // else names a Lucide icon.
  import Users from '@lucide/svelte/icons/users'
  import Search from '@lucide/svelte/icons/search'
  import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal'
  import CircleArrowDown from '@lucide/svelte/icons/circle-arrow-down'
  import Play from '@lucide/svelte/icons/play'
  import Square from '@lucide/svelte/icons/square'
  import RotateCw from '@lucide/svelte/icons/rotate-cw'
  import X from '@lucide/svelte/icons/x'
  import Check from '@lucide/svelte/icons/check'
  import Plus from '@lucide/svelte/icons/plus'
  import UserPlus from '@lucide/svelte/icons/user-plus'
  import UserX from '@lucide/svelte/icons/user-x'
  import Undo2 from '@lucide/svelte/icons/undo-2'
  import ExternalLink from '@lucide/svelte/icons/external-link'
  import ChevronRight from '@lucide/svelte/icons/chevron-right'
  import ChevronLeft from '@lucide/svelte/icons/chevron-left'
  import Sun from '@lucide/svelte/icons/sun'
  import Moon from '@lucide/svelte/icons/moon'
  import SunMoon from '@lucide/svelte/icons/sun-moon'
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import Info from '@lucide/svelte/icons/info'
  import Copy from '@lucide/svelte/icons/copy'
  import FileText from '@lucide/svelte/icons/file-text'
  import Code from '@lucide/svelte/icons/code'
  import Image from '@lucide/svelte/icons/image'
  import Mic from '@lucide/svelte/icons/mic'
  import Ellipsis from '@lucide/svelte/icons/ellipsis'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import Ban from '@lucide/svelte/icons/ban'
  import PanelLeftOpen from '@lucide/svelte/icons/panel-left-open'
  import PanelLeftClose from '@lucide/svelte/icons/panel-left-close'
  import type { Component } from 'svelte'

  const glyphs = {
    people: Users,
    search: Search,
    settings: SlidersHorizontal,
    // macOS brings updates in with an arrow pointing down, so does
    // FrameFairy.
    update: CircleArrowDown,
    // A run starts and stops the way anything that plays does.
    run: Play,
    stop: Square,
    refresh: RotateCw,
    close: X,
    x: X,
    check: Check,
    plus: Plus,
    connect: UserPlus,
    'not-them': UserX,
    undo: Undo2,
    external: ExternalLink,
    chevron: ChevronRight,
    back: ChevronLeft,
    sun: Sun,
    moon: Moon,
    auto: SunMoon,
    alert: TriangleAlert,
    info: Info,
    copy: Copy,
    doc: FileText,
    code: Code,
    image: Image,
    mic: Mic,
    more: Ellipsis,
    trash: Trash2,
    exclude: Ban,
    // The button at the top of the sidebar, as FrameFairy draws it: a
    // panel opening while the sidebar is not kept, closing while it is.
    'sidebar-keep': PanelLeftOpen,
    'sidebar-close': PanelLeftClose,
  } satisfies Record<string, Component<any>>

  export type IconName = keyof typeof glyphs
</script>

<script lang="ts">
  // Lucide's own way of drawing: a line of the same weight at every size,
  // as FrameFairy draws it.
  let { name, size = 16, line = 1.75 }: { name: IconName; size?: number; line?: number } = $props()

  const Glyph = $derived(glyphs[name])
</script>

<Glyph {size} strokeWidth={line} absoluteStrokeWidth aria-hidden="true" focusable="false" class="icon" />

<style>
  :global(svg.icon) { flex: none; display: block; }
</style>
