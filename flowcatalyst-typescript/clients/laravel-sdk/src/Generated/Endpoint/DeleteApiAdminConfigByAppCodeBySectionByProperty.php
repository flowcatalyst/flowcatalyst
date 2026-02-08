<?php

namespace FlowCatalyst\Generated\Endpoint;

class DeleteApiAdminConfigByAppCodeBySectionByProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $property;
    protected $section;
    /**
     * @param string $appCode
     * @param string $property
     * @param string $section
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     */
    public function __construct(string $appCode, string $property, string $section, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->property = $property;
        $this->section = $section;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'DELETE';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{property}', '{section}'], [$this->appCode, $this->property, $this->section], '/api/admin/config/{appCode}/{section}/{property}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['clientId', 'scope']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        $optionsResolver->addAllowedTypes('scope', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (204 === $status) {
            return null;
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyForbiddenException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyNotFoundException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}