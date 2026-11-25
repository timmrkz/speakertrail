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
  import Minus from '@lucide/svelte/icons/minus'
  import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down'
  import Download from '@lucide/svelte/icons/download'
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
  import PenLine from '@lucide/svelte/icons/pen-line'
  import FileText from '@lucide/svelte/icons/file-text'
  import Code from '@lucide/svelte/icons/code'
  import Image from '@lucide/svelte/icons/image'
  import Film from '@lucide/svelte/icons/film'
  import Mic from '@lucide/svelte/icons/mic'
  import Bug from '@lucide/svelte/icons/bug'
  import Paperclip from '@lucide/svelte/icons/paperclip'
  import Ellipsis from '@lucide/svelte/icons/ellipsis'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import Folder from '@lucide/svelte/icons/folder'
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
    // Neither for nor against, beside a tick and a cross.
    neither: Minus,
    plus: Plus,
    connect: UserPlus,
    'not-them': UserX,
    undo: Undo2,
    external: ExternalLink,
    chevron: ChevronRight,
    // The mark of a list to pick from, and of a model still to download
    // in it, as FrameFairy draws them.
    pick: ChevronsUpDown,
    download: Download,
    back: ChevronLeft,
    sun: Sun,
    moon: Moon,
    auto: SunMoon,
    alert: TriangleAlert,
    info: Info,
    copy: Copy,
    // Draft, which writes a first message to a person.
    draft: PenLine,
    doc: FileText,
    code: Code,
    image: Image,
    // A screen recording on Report a Problem, which shows no picture.
    film: Film,
    // Dictate on Report a Problem.
    mic: Mic,
    // Bugs in the sidebar, the mark issue trackers draw for a bug report,
    // and the paperclip that attaches pictures to it, as in Mail.
    report: Bug,
    attach: Paperclip,
    more: Ellipsis,
    trash: Trash2,
    // Shows a file in Finder, as FrameFairy draws it.
    folder: Folder,
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
