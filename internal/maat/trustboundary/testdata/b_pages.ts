// Class B: per-item page emission inside a loop.
export async function attach(pdf: { addPage(): void }, attachments: string[]) {
  attachments.forEach((a) => {
    pdf.addPage(); // want B
  });
  for (const a of attachments) {
    pdf.addPage(); // want B
  }
}
