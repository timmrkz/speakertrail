import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { listen } from './lib/said'

// What the window says and what the person does go into the app's log
// from the first moment, see lib/said.ts.
listen()

const app = mount(App, { target: document.getElementById('app')! })

export default app
