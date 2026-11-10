- **fix(maat):** a conflict check that finds foreign activity in a live quiet
  or loaded reservation now marks the measurement contaminated
  (`invalidated_by`) and keeps the holder on the machine. Before, it ended the
  reservation, which freed the machine for the next lane in the middle of the
  holder's window. On 2026-11-03 this ended Mercury's M5/Studio release gate,
  with no release on record. A reservation whose window has not started yet is
  still ended, as before.
