import { updateDoc } from "firebase/firestore";
// Class F: sensitive answers on a client-readable document.
export async function store(ref: unknown, answers: Record<string, string>) {
  await updateDoc(ref as never, { // want F
    title: "Petition",
    formAnswers: answers,
  });
}
