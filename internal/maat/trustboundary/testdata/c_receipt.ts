// Class C: client-sent provenance on a request type.
export interface SignRequest {
  pdf: string;
  receipt: string; // want C
  generatedBy: string; // want C
}
