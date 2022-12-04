package config

// Represents an object that will be populated from the configuration resources
type ConfigObject[T any] struct {
	o          T
	objectName string
}

// If the object implements this interface, this method will be executed after each
// updatet, typically for cooking the derived attributes
type Initializable interface {
	initialize() error
}

// Creates an uninitialized configuration object
func NewConfigObject[T any](name string) *ConfigObject[T] {
	var co ConfigObject[T]
	co.objectName = name
	return &co
}

// Reads the configuration from the associated resource and initializes it,
// if an initialize() method is defined
func (co *ConfigObject[T]) Update(cm *ConfigurationManager) error {

	var theObject T
	if err := cm.BuildJSONConfigObject(co.objectName, &theObject); err != nil {
		return err
	} else {
		if initializable, ok := any(theObject).(Initializable); ok {
			if err := initializable.initialize(); err != nil {
				return err
			}
		}
		co.o = theObject
		return nil
	}
}

// Provides access to the configuration object. Returns a copy, so the underlying
// object may be modified safely
func (co *ConfigObject[T]) Get() T {
	return co.o
}
