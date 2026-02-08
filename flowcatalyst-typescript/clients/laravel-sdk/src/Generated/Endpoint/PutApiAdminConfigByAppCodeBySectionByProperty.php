<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiAdminConfigByAppCodeBySectionByProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $property;
    protected $section;
    /**
     * @param string $appCode
     * @param string $property
     * @param string $section
     * @param null|\FlowCatalyst\Generated\Model\SetConfigRequest $requestBody
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     */
    public function __construct(string $appCode, string $property, string $section, ?\FlowCatalyst\Generated\Model\SetConfigRequest $requestBody = null, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->property = $property;
        $this->section = $section;
        $this->body = $requestBody;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{property}', '{section}'], [$this->appCode, $this->property, $this->section], '/api/admin/config/{appCode}/{section}/{property}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\SetConfigRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
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
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
        if (201 === $status) {
            return null;
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}