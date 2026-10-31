### Fixed

- Allow up to one second for the native app’s version probe after the observed cold-start response exceeded 200 ms. Other executable probes and pipe-drain bounds remain 200 ms; caller cancellation and the three-second inactive recheck deadline remain in force.
