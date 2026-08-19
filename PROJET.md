# SyncCal - Synchroniseur de calendrier par caldav

## Objectif

Synchroniser un calendrier vers un ou plusieurs autres dans nextcloud et carbonio.

## Obligation

- La synchronisation se fait, suivant les possibilités, heure par heure, ou au changement si c'est détectable sans surcharge du serveur,
- Le calendrier de départ peut être public sans authentification OU un calendrier authentifié (login/mot de passe ou token),
- Le calendrier d'arrivée peut se trouver sur un compte avec login et mot de passe, 
- On préférera toujours un token à un mot de passe de compte (carbonio ou nextcloud le font.)

## Analyse
