// Wails event-bus helpers. The variadic-args contract — `Emit
// ...interface{}` on the Go side, `EventsOn(name, ...args)`
// on the TS side — is brittle because each subscriber must
// remember to unwrap `args[0]`. This module pins the contract
// in one place so future subscribers can write
//
//   useEffect(() => {
//     return subscribe<InfoUpdated>('info-updated', (p) => { ... });
//   }, []);
//
// and rely on the helper to do the unwrap + cleanup wiring.
//
// The Go side emits with a single payload arg via
// `app.emitter.Emit(ctx, name, payload)`. Subscribers receive
// `(args) => void` where args is the JS array Wails constructs
// from each variadic Go arg.

import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';

/** subscribe registers `h` as a listener for the Wails event
 *  `name` and returns a teardown function. The payload type
 *  `T` is what the Go side passed to Emit — typically a
 *  generated `models.*` class or a plain interface. */
export function subscribe<T>(name: string, h: (payload: T) => void): () => void {
	const off = EventsOn(name, (...args: unknown[]) => {
		h(args[0] as T);
	});
	return () => EventsOff(name);
}
