import { setDoc } from "firebase/firestore";
// Class E: merge-write of a spread object.
export async function save(ref: unknown, prev: Record<string, unknown>, next: Record<string, unknown>) {
  await setDoc(ref as never, { ...prev, ...next }, { merge: true }); // want E
}
