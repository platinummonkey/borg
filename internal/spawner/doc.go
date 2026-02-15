// Package spawner provides an interface for launching and managing agent
// processes across different execution environments.
//
// The [Spawner] interface defines a lifecycle-based contract (PreSpawn, Spawn,
// PostSpawn, PreStop, Stop, PostStop) with concrete implementations for
// local processes ([LocalSpawner]), SSH ([SSHSpawner]), and Docker
// ([DockerSpawner]).
package spawner
